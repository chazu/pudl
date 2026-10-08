package importer

import (
	"context"
	"fmt"
	"github.com/chazu/pudl/internal/artifacts"
	"os"
	"path/filepath"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/validator"
)

// EnhancedImporter imports files into the catalog with content-based identity.
//
// It used to embed a separate `Importer` type. Nothing outside this package ever
// constructed one, so the split was pure layering — and it was the layering that
// hid the memory problem: the call that accumulated every decoded record into one
// slice lived in the embedded type, while the pipeline above it read as though it
// streamed.
type EnhancedImporter struct {
	dataPath    string
	schemaPath  string   // primary schema path (first in schemaPaths)
	schemaPaths []string // all schema paths in priority order
	catalogDB   *database.CatalogDB
	inferrer    *inference.SchemaInferrer
	chain       *validator.ChainValidator // built on first manual-schema import
	redactor    *redact.Registry          // sensitive-field paths per schema
	projector   *projection.Registry      // schema-declared fact projections
}

// NewEnhancedImporter creates a new enhanced importer with content-based ID support.
// The schemaPath parameter is the primary schema path. For multi-path support,
// use NewEnhancedImporterWithSchemaPaths.
func NewEnhancedImporter(dataPath, schemaPath, configDir string) (*EnhancedImporter, error) {
	return NewEnhancedImporterWithSchemaPaths(dataPath, configDir, schemaPath)
}

// NewEnhancedImporterWithSchemaPaths creates a new enhanced importer with multiple schema paths.
// Paths are searched in order; earlier paths take priority (per-repo shadows global).
func NewEnhancedImporterWithSchemaPaths(dataPath, configDir string, schemaPaths ...string) (*EnhancedImporter, error) {
	return newImporterState(dataPath, configDir, schemaPaths...)
}

// ImportFileWithFriendlyIDs imports a file using content-based ID generation
func (e *EnhancedImporter) ImportFileWithFriendlyIDs(opts ImportOptions) (*ImportResult, error) {
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	var err error
	opts.Limits, err = opts.Limits.Resolve()
	if err != nil {
		return nil, err
	}
	if err := opts.Context.Err(); err != nil {
		return nil, err
	}
	// Ensure basic schemas exist
	if err := e.ensureBasicSchemas(); err != nil {
		return nil, fmt.Errorf("failed to ensure basic schemas: %w", err)
	}

	fileInfo, err := os.Stat(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}
	if fileInfo.IsDir() {
		return nil, fmt.Errorf("%s is a directory; import its files individually or with a wildcard", opts.SourcePath)
	}

	plainOwnedBytes := int64(0)
	// A compressed source is decompressed once, up front, and every later step —
	// format detection, hashing, storage, decoding — reads the plain bytes. The
	// original path still names the import (origin, reported source).
	if compression := DetectCompression(opts.SourcePath); compression != "none" {
		tempDir, err := e.TempDir()
		if err != nil {
			return nil, err
		}
		plainPath, err := DecompressToLimited(opts.Context, opts.SourcePath, tempDir, min(opts.Limits.DecodedBytes, opts.Limits.StagingBytes/2))
		if err != nil {
			return nil, fmt.Errorf("failed to decompress %s: %w", opts.SourcePath, err)
		}
		defer os.Remove(plainPath)
		opts.OriginPath = opts.originPath()
		opts.SourcePath = plainPath
		if info, err := os.Stat(plainPath); err == nil {
			plainOwnedBytes = info.Size()
		} else {
			return nil, err
		}
	}

	format, err := e.detectFormat(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect format: %w", err)
	}

	// Detect origin if not provided
	origin := opts.Origin
	if origin == "" {
		origin = e.detectOrigin(opts.originPath(), format)
	}

	// Rewrite the source first when records must change (--set, redaction).
	var transformed *transformResult
	if e.needsTransform(opts) {
		shape := sourceShape{format: format, origin: origin}
		shape.collection, _ = streamableCollectionFormat(opts.SourcePath, format)
		transformed, err = e.transformSource(opts, opts.SourcePath, shape)
		if err != nil {
			return nil, err
		}
		defer transformed.cleanup()
		if transformed.path != "" {
			opts.OriginPath = opts.originPath()
			opts.SourcePath = transformed.path
			plainOwnedBytes += transformed.size
			// --set may grow records; the rewritten file, not the original,
			// is what the main pass decodes.
			opts.Limits.DecodedBytes = max(opts.Limits.DecodedBytes, transformed.size)
		}
		if transformed.assignments != "" {
			reader, err := openAssignments(transformed.assignments)
			if err != nil {
				return nil, err
			}
			defer reader.close()
			opts.assignments = reader
		}
	}

	// Generate timestamp for metadata
	timestamp := time.Now()

	// Create date-based directory structure up front: staging writes into it.
	dateDir := timestamp.Format("2006/01/02")
	rawDir := filepath.Join(e.dataPath, "raw", dateDir)
	metadataDir := filepath.Join(e.dataPath, "metadata")
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Stage and hash in one pass. The import used to hash the file, then decode
	// it, then copy it — three reads of the same bytes, two of them for purposes
	// an io.MultiWriter serves at once. Identity is SHA256 of the (decompressed)
	// source bytes, taken as read rather than from anything decoded.
	//
	// Writing to a temp file and renaming is also what makes staging atomic: a
	// killed import leaves a temp file, not a half-written record in the raw tree
	// that a later read would treat as evidence.
	staged, err := stageSourceLimited(opts.Context, opts.SourcePath, rawDir, min(opts.Limits.DecodedBytes, opts.Limits.StagingBytes-plainOwnedBytes))
	if err != nil {
		return nil, err
	}
	defer staged.Discard() // no-op once committed
	if plainOwnedBytes > 0 {
		if err := os.Remove(opts.SourcePath); err != nil {
			return nil, err
		}
	}

	contentHash := staged.ContentHash
	mainID := contentHash

	storedPath := filepath.Join(rawDir, fmt.Sprintf("%s%s", mainID[:16], filepath.Ext(opts.SourcePath)))
	var stream *collectionStream
	if collectionFormat, ok := streamableCollectionFormat(staged.Path(storedPath), format); ok {
		opts.collectionFormat = format
		opts.Limits.StagingBytes -= staged.Size
		stream = &collectionStream{importer: e, opts: opts, collectionID: mainID, timestamp: timestamp, rawDir: rawDir, metadataDir: metadataDir, tally: projection.NewTally()}
		if err := stream.prepare(staged.Path(storedPath), collectionFormat); err != nil {
			return nil, err
		}
		defer stream.close()
	}
	if stream == nil && staged.Size > opts.Limits.RecordBytes {
		return nil, fmt.Errorf("document exceeds record byte limit (%d)", opts.Limits.RecordBytes)
	}
	tempDir, err := e.TempDir()
	if err != nil {
		return nil, err
	}
	var result *ImportResult
	err = artifacts.WithLock(opts.Context, e.catalogDB.Root(), func() error {
		existing, err := e.catalogDB.FindByContentHash(contentHash)
		if err != nil {
			return err
		}
		if existing != nil {
			result = existingImportResult(existing, opts.originPath())
			if opts.Explain {
				result.Explanation = loadOriginalExplanation(existing.MetadataPath)
			}
			// Already cataloged, but this is still an observation: an explicit
			// schema may now apply, and the records are again their resources'
			// latest state.
			if stream != nil {
				result.Reassigned, err = stream.reobserveAll()
				applyTally(result, stream.tally)
			} else if opts.ManualSchema != "" || e.projector.For(existing.Schema) != nil {
				result.Reassigned, err = e.reobserveDocument(opts, existing, staged.Path(storedPath), format, origin)
			}
			return err
		}
		journal, err := artifacts.NewJournalContext(opts.Context, tempDir)
		if err != nil {
			return err
		}
		defer journal.Close()
		opts.publication = journal
		if err := journal.Publish(staged.Path(storedPath), storedPath); err != nil {
			return err
		}
		if stream != nil {
			stream.opts.publication = journal
			result, err = stream.commit(origin, storedPath, staged.Size, journal)
		} else {
			opts.OriginPath = opts.originPath()
			opts.SourcePath = storedPath
			result, err = e.importDocument(opts, documentImport{id: mainID, contentHash: contentHash, format: format, origin: origin, timestamp: timestamp, storedPath: storedPath, metadataDir: metadataDir, sizeBytes: staged.Size})
		}
		if err != nil {
			journal.Rollback()
			return err
		}
		result.SourcePath = opts.originPath()
		return nil
	})
	if result != nil && transformed != nil {
		result.Redacted = transformed.redacted
	}
	return result, err
}

// CatalogDB returns the catalog the importer writes to, so callers recording
// related rows (item schemas) reuse the open connection.
func (e *EnhancedImporter) CatalogDB() *database.CatalogDB {
	return e.catalogDB
}

// TempDir returns the workspace-local scratch directory for import
// intermediates (decompressed sources, envelope payloads), creating it on
// demand. Keeping them under the data directory avoids writing beside the
// user's source file and keeps them on the same filesystem as raw storage.
func (e *EnhancedImporter) TempDir() (string, error) {
	dir := filepath.Join(e.dataPath, "tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create import temp directory: %w", err)
	}
	return dir, nil
}

// GetIDDisplayFormat returns a proquint display format for a content hash ID
func (e *EnhancedImporter) GetIDDisplayFormat(id string) string {
	return idgen.HashToProquint(id)
}

// getSchemaIdentityFields returns the identity fields for a schema from schema metadata.
// Returns nil if the schema has no identity fields or is a catchall/fallback schema.
func (e *EnhancedImporter) getSchemaIdentityFields(schema string) []string {
	if e.inferrer == nil {
		return nil
	}
	meta, found := e.inferrer.GetSchemaMetadata(schema)
	if !found {
		return nil
	}
	return meta.IdentityFields
}

// identityNamespace returns the schema used to namespace resource identity for
// the given assigned schema: the root of its inheritance family. Falls back to
// the schema itself when the inferrer/graph is unavailable.
func (e *EnhancedImporter) identityNamespace(schema string) string {
	if e.inferrer == nil {
		return schema
	}
	return e.inferrer.GetInheritanceGraph().IdentityRoot(schema)
}

// createCollectionEntryWithContentHash creates the main collection catalog entry with content hash IDs
func (e *EnhancedImporter) createCollectionEntryIn(w database.CatalogWriter, opts ImportOptions, timestamp time.Time, origin, collectionID, storedPath, metadataDir string, sizeBytes int64, recordCount int) (*ImportResult, error) {
	schema := schemaname.Normalize("pudl.schemas/pudl/core:#Collection")
	confidence := 0.8
	format := opts.collectionFormat
	if format == "" {
		format = "ndjson"
	}
	contentHash := collectionID // For collections, content hash is the collection ID (file hash)

	// Collections use catchall identity (no identity fields)
	resourceID := identity.ComputeResourceID(e.identityNamespace(schema), nil, contentHash)
	version := 1

	metadata := &ImportMetadata{
		ID: collectionID,
		SourceInfo: SourceInfo{
			OriginalPath: opts.SourcePath,
			Origin:       origin,
			Confidence:   "high",
		},
		ImportMetadata: ImportMeta{
			Format:      format,
			RecordCount: recordCount,
			SizeBytes:   sizeBytes,
			Timestamp:   timestamp.Format(time.RFC3339),
		},
		SchemaInfo: SchemaInfo{
			CuePackage:       extractPackage(schema),
			CueDefinition:    schema,
			ValidationStatus: "auto-assigned",

			SchemaVersion: "v1.0",
		},
		ResourceTracking: ResourceTracking{
			IdentityFields: []string{},
			TrackedFields:  []string{"item_count", "item_schemas"},
			ResourceID:     resourceID,
			ContentHash:    contentHash,
			Version:        version,
		},
	}

	metadataPath := filepath.Join(metadataDir, collectionID+".meta")
	if err := e.publishMetadata(*metadata, metadataPath, opts); err != nil {
		return nil, fmt.Errorf("failed to save collection metadata: %w", err)
	}

	collectionType := "collection"
	entry := database.CatalogEntry{
		ID:              metadata.ID,
		StoredPath:      storedPath,
		MetadataPath:    metadataPath,
		ImportTimestamp: timestamp,
		Format:          format,
		Origin:          origin,
		Schema:          schema,
		Confidence:      confidence,
		RecordCount:     recordCount,
		SizeBytes:       sizeBytes,
		CollectionType:  &collectionType,
		ResourceID:      &resourceID,
		ContentHash:     &contentHash,
		Version:         &version,
	}

	if err := w.AddEntry(entry); err != nil {
		return nil, fmt.Errorf("failed to add collection to catalog: %w", err)
	}

	return &ImportResult{
		ID:               metadata.ID,
		SourcePath:       opts.SourcePath,
		StoredPath:       storedPath,
		MetadataPath:     metadataPath,
		DetectedFormat:   format,
		DetectedOrigin:   origin,
		AssignedSchema:   schema,
		SchemaConfidence: confidence,
		RecordCount:      recordCount,
		SizeBytes:        sizeBytes,
		ImportTimestamp:  timestamp.Format(time.RFC3339),
		ResourceID:       resourceID,
		ContentHash:      contentHash,
		Version:          version,
	}, nil
}
