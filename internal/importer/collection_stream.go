package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/schemaname"
)

// collectionStream is the streamed import of a record collection — NDJSON or a
// top-level JSON array.
//
// The path it replaces decoded every record into one slice, wrote the collection
// entry, then walked the slice writing items. Peak memory was the whole decoded
// record set, and a failure part-way through unwound by hand through
// cleanupFailedCollectionImport, reconstructing item paths from an index.
//
// Now: records arrive one at a time from the decoder, and the catalog writes are
// one transaction. The record count is only known when the stream ends, so items
// stream first and the collection entry lands last with the true count — which
// also removes the window in which a collection row exists describing items that
// were never written.
type collectionStream struct {
	importer     *EnhancedImporter
	opts         ImportOptions
	collectionID string
	timestamp    time.Time
	rawDir       string
	metadataDir  string

	// stagedFiles are the item artifacts written so far. A rollback cannot
	// unwrite a file, so they are removed after the transaction aborts: an orphan
	// file is wasted disk, an orphan row is a lie.
	stagedFiles []string
}

// run decodes the staged source and writes every item plus the collection entry
// inside one transaction.
func (c *collectionStream) run(sourcePath, format, origin, storedPath string, sizeBytes int64) (*ImportResult, error) {
	var (
		result      *ImportResult
		recordCount int
	)

	err := c.importer.catalogDB.WithCatalogTx(func(tx *database.CatalogTx) error {
		c.stagedFiles = nil

		source, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("open staged source: %w", err)
		}
		defer source.Close()

		sink := func(index int, raw json.RawMessage) error {
			return c.writeItem(tx, index, raw)
		}

		switch format {
		case "ndjson":
			recordCount, err = streamNDJSON(source, sink)
		case "json-array":
			recordCount, err = streamJSONArray(source, sink)
		default:
			return fmt.Errorf("collectionStream: unsupported format %q", format)
		}
		if err != nil {
			return err
		}
		if recordCount == 0 {
			return fmt.Errorf("no records found in %s", c.opts.SourcePath)
		}

		result, err = c.importer.createCollectionEntryIn(tx, c.opts, c.timestamp, origin,
			c.collectionID, storedPath, c.metadataDir, sizeBytes, recordCount)
		return err
	})

	if err != nil {
		// The rows are already rolled back; only the files need removing.
		for _, path := range c.stagedFiles {
			_ = os.Remove(path)
		}
		return nil, err
	}
	return result, nil
}

// writeItem records one streamed record as a collection item.
func (c *collectionStream) writeItem(tx *database.CatalogTx, index int, raw json.RawMessage) error {
	e := c.importer

	// Identity is the hash of the record's canonical JSON, unchanged from the
	// path this replaces.
	var itemData interface{}
	if err := json.Unmarshal(raw, &itemData); err != nil {
		return fmt.Errorf("decode record %d: %w", index, err)
	}
	canonical, err := json.Marshal(itemData)
	if err != nil {
		return fmt.Errorf("marshal record %d: %w", index, err)
	}
	itemContentHash := idgen.ComputeContentID(canonical)

	// Content-addressed dedup: a record already in the catalog gains a membership
	// rather than a second copy. Read through the transaction, so a repeated
	// record *within this import* deduplicates against itself.
	existing, err := tx.FindByContentHash(itemContentHash)
	if err != nil {
		return fmt.Errorf("check for existing item %d: %w", index, err)
	}
	if existing != nil {
		return tx.AddCollectionMembership(c.collectionID, existing.ID, index)
	}

	itemFilename := fmt.Sprintf("%s_item_%d", c.collectionID, index)
	itemPath := filepath.Join(c.rawDir, itemFilename+".json")
	stored, err := json.MarshalIndent(itemData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal record %d for storage: %w", index, err)
	}
	if err := os.WriteFile(itemPath, stored, 0o644); err != nil {
		return fmt.Errorf("write item %d: %w", index, err)
	}
	c.stagedFiles = append(c.stagedFiles, itemPath)

	schema, confidence := e.assignItemSchema(itemData, c.opts)
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

	latestVersion, err := tx.GetLatestVersion(resourceID)
	if err != nil {
		return fmt.Errorf("get latest version for item %d: %w", index, err)
	}
	version := latestVersion + 1

	itemMetadata := ImportMetadata{
		ID: itemContentHash,
		SourceInfo: SourceInfo{
			OriginalPath: c.opts.SourcePath,
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
	itemMetadataPath := filepath.Join(c.metadataDir, itemFilename+".meta")
	if err := e.saveMetadata(itemMetadata, itemMetadataPath); err != nil {
		return fmt.Errorf("save item %d metadata: %w", index, err)
	}
	c.stagedFiles = append(c.stagedFiles, itemMetadataPath)

	collectionType := "item"
	itemID := itemContentHash
	var identityJSONPtr *string
	if identityJSON != "" {
		identityJSONPtr = &identityJSON
	}

	return tx.AddEntry(database.CatalogEntry{
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
	})
}
