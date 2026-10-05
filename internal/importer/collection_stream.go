package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/ingestprep"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/schemaname"
)

// collectionStream prepares one record at a time into private files and a
// descriptor spool. Only authoritative dedup/version allocation and publication
// run inside the atomic catalog commit.
type collectionStream struct {
	importer            *EnhancedImporter
	opts                ImportOptions
	collectionID        string
	timestamp           time.Time
	rawDir, metadataDir string
	dir                 string
	spool               *os.File
	recordCount         int
	stagedBytes         int64
	explanations        []ItemExplanation
	truncated           bool
}
type preparedItem struct {
	Entry    database.CatalogEntry
	Metadata ImportMetadata
}

func (c *collectionStream) close() {
	if c.spool != nil {
		_ = c.spool.Close()
	}
	_ = os.RemoveAll(c.dir)
}
func (c *collectionStream) prepare(sourcePath, format string) (resultErr error) {
	tmp, err := c.importer.TempDir()
	if err != nil {
		return err
	}
	c.dir, err = os.MkdirTemp(tmp, "collection-prepare-*")
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			c.close()
		}
	}()
	c.spool, err = os.Create(filepath.Join(c.dir, "entries.ndjson"))
	if err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	reader := &ingestprep.Reader{Context: c.opts.Context, Source: source, Remaining: c.opts.Limits.DecodedBytes}
	sink := func(index int, raw json.RawMessage) error { return c.prepareItem(index, raw) }
	switch format {
	case "ndjson":
		c.recordCount, err = ingestprep.NDJSON(reader, c.opts.Limits.RecordBytes, sink)
	case "json-array":
		c.recordCount, err = ingestprep.Array(reader, c.opts.Limits.RecordBytes, sink)
	default:
		return fmt.Errorf("unsupported collection format %q", format)
	}
	if err != nil {
		return err
	}
	if c.recordCount == 0 {
		return fmt.Errorf("no records found in %s", c.opts.SourcePath)
	}
	return nil
}

// commit is called with the workspace artifact lock held.
func (c *collectionStream) commit(origin, storedPath string, sizeBytes int64, journal *artifacts.Journal) (*ImportResult, error) {
	if _, err := c.spool.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var result *ImportResult
	err := c.importer.catalogDB.WithCatalogTxContext(c.opts.Context, func(tx *database.CatalogTx) error {
		dec := json.NewDecoder(c.spool)
		dec.UseNumber()
		for {
			var item preparedItem
			if err := dec.Decode(&item); err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			entry := item.Entry
			trace := item.Metadata.SchemaInfo.Explanation
			existing, err := tx.FindByContentHash(*entry.ContentHash)
			if err != nil {
				return err
			}
			if existing != nil {
				if c.opts.Explain {
					trace = loadOriginalExplanation(existing.MetadataPath)
					c.appendExplanation(*entry.ItemIndex, trace)
				}
				if err := tx.AddCollectionMembership(c.collectionID, existing.ID, *entry.ItemIndex); err != nil {
					return err
				}
				continue
			}
			latest, err := tx.GetLatestVersion(*entry.ResourceID)
			if err != nil {
				return err
			}
			version := latest + 1
			entry.Version = &version
			item.Metadata.ResourceTracking.Version = version
			rawDestination := filepath.Join(c.rawDir, filepath.Base(entry.StoredPath))
			if err := journal.Publish(entry.StoredPath, rawDestination); err != nil {
				return err
			}
			entry.StoredPath = rawDestination
			if err := c.importer.publishMetadata(item.Metadata, entry.MetadataPath, c.opts); err != nil {
				return err
			}
			c.appendExplanation(*entry.ItemIndex, trace)

			if err := tx.AddEntry(entry); err != nil {
				return err
			}
		}
		var err error
		result, err = c.importer.createCollectionEntryIn(tx, c.opts, c.timestamp, origin, c.collectionID, storedPath, c.metadataDir, sizeBytes, c.recordCount)
		return err
	})
	if result != nil {
		result.ItemExplanations = c.explanations
		result.ExplanationsTruncated = c.truncated
	}
	return result, err
}

// writeItem records one streamed record as a collection item.
func (c *collectionStream) prepareItem(index int, raw json.RawMessage) error {
	if err := c.opts.Context.Err(); err != nil {
		return err
	}
	e := c.importer

	// Decode without rounding numbers through float64: an integer beyond 2^53
	// must reach inference, identity and storage with every digit intact.
	itemData, err := idgen.DecodeJSONExact(raw)
	if err != nil {
		return fmt.Errorf("decode record %d: %w", index, err)
	}
	// Identity is the hash of the record's canonical JSON. For records whose
	// numbers survive a float64 round trip the canonical bytes equal those of
	// the earlier float64 path, so re-imports still deduplicate against items
	// stored before.
	canonical, err := json.Marshal(itemData)
	if err != nil {
		return fmt.Errorf("marshal record %d: %w", index, err)
	}
	itemContentHash := idgen.ComputeContentID(canonical)

	itemFilename := fmt.Sprintf("%s_item_%d", c.collectionID, index)
	itemPath := filepath.Join(c.dir, itemFilename+".json")
	// Store the record as it arrived (indented), not a re-encoding of the
	// decoded value, so retained evidence keeps the source's exact numbers.
	var indented bytes.Buffer
	if err := json.Indent(&indented, bytes.TrimSpace(raw), "", "  "); err != nil {
		return fmt.Errorf("format record %d for storage: %w", index, err)
	}
	stored := indented.Bytes()
	if c.stagedBytes+int64(len(stored)) > c.opts.Limits.StagingBytes {
		return fmt.Errorf("collection exceeds staging byte limit")
	}
	if err := os.WriteFile(itemPath, stored, 0o644); err != nil {
		return fmt.Errorf("write item %d: %w", index, err)
	}

	assigned := e.assignItemSchemaDetailed(itemData, c.opts)
	schema, confidence := assigned.Schema, assigned.Confidence
	schemaIdentityFields := e.getSchemaIdentityFields(schema)
	identityValues, extractErr := identity.ExtractFieldValues(itemData, schemaIdentityFields)
	if extractErr != nil {
		identityValues = nil
	}
	resourceID := identity.ComputeResourceID(e.identityNamespace(schema), identityValues, itemContentHash)

	identityJSON := ""
	if len(identityValues) > 0 {
		if canonicalIdentity, err := identity.CanonicalIdentityJSON(identityValues); err == nil {
			identityJSON = canonicalIdentity
		}
	}

	version := 0

	itemMetadata := ImportMetadata{
		ID: itemContentHash,
		SourceInfo: SourceInfo{
			OriginalPath: c.opts.originPath(),
			Origin:       fmt.Sprintf("%s_item_%d", c.collectionID, index),
			Confidence:   "high",
		},
		ImportMetadata: ImportMeta{
			Format:      "json",
			RecordCount: 1,
			SizeBytes:   int64(len(stored)),
			Timestamp:   c.timestamp.Format(time.RFC3339),
		},
		SchemaInfo: SchemaInfo{
			CuePackage:       extractPackage(schema),
			CueDefinition:    schema,
			ValidationStatus: "auto-assigned",
			SchemaVersion:    "v1.0",
		},
		ResourceTracking: ResourceTracking{
			IdentityFields: schemaIdentityFields,
			TrackedFields:  []string{},
			ResourceID:     resourceID,
			ContentHash:    itemContentHash,
			IdentityValues: identityValues,
			Version:        version,
		},
	}
	enrichAssignment(&itemMetadata.SchemaInfo, nil, assigned)
	itemMetadataPath := filepath.Join(c.metadataDir, itemFilename+".meta")
	collectionType := "item"
	itemID := itemContentHash
	var identityJSONPtr *string
	if identityJSON != "" {
		identityJSONPtr = &identityJSON
	}

	entry := database.CatalogEntry{
		ID:              itemID,
		StoredPath:      itemPath,
		MetadataPath:    itemMetadataPath,
		ImportTimestamp: c.timestamp,
		Format:          "json",
		Origin:          fmt.Sprintf("%s_item_%d", c.collectionID, index),
		Schema:          schemaname.Normalize(schema),
		Confidence:      confidence,
		RecordCount:     1,
		SizeBytes:       int64(len(stored)),
		CollectionID:    &c.collectionID,
		ItemIndex:       &index,
		CollectionType:  &collectionType,
		ItemID:          &itemID,
		ResourceID:      &resourceID,
		ContentHash:     &itemContentHash,
		IdentityJSON:    identityJSONPtr,
		Version:         &version,
	}
	prepared := preparedItem{Entry: entry, Metadata: itemMetadata}
	descriptor, err := json.Marshal(prepared)
	if err != nil {
		return err
	}
	c.stagedBytes += int64(len(stored) + len(descriptor) + 1)
	if c.stagedBytes > c.opts.Limits.StagingBytes {
		return fmt.Errorf("collection exceeds staging byte limit")
	}
	_, err = c.spool.Write(append(descriptor, '\n'))
	return err
}

func (c *collectionStream) appendExplanation(index int, trace *inference.InferenceTrace) {
	if !c.opts.Explain {
		return
	}
	if len(c.explanations) < 32 {
		c.explanations = append(c.explanations, ItemExplanation{Index: index, Trace: trace})
	} else {
		c.truncated = true
	}
}
