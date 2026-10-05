package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/schemaname"
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

	// A compressed source is decompressed once, up front, and every later step —
	// format detection, hashing, storage, decoding — reads the plain bytes. The
	// original path still names the import (origin, reported source).
	if compression := DetectCompression(opts.SourcePath); compression != "none" {
		tempDir, err := e.TempDir()
		if err != nil {
			return nil, err
		}
		plainPath, err := DecompressTo(opts.SourcePath, tempDir)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress %s: %w", opts.SourcePath, err)
		}
		defer os.Remove(plainPath)
		opts.OriginPath = opts.originPath()
		opts.SourcePath = plainPath
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
	staged, err := stageSource(opts.SourcePath, rawDir)
	if err != nil {
		return nil, err
	}
	defer staged.Discard() // no-op once committed

	contentHash := staged.ContentHash
	mainID := contentHash

	// Content hash dedup: check if this exact content exists anywhere in the catalog
	existingEntry, err := e.catalogDB.FindByContentHash(contentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing content: %w", err)
	}
	if existingEntry != nil {
		// The staged bytes are already in the catalog; the deferred Discard
		// removes the copy just written.
		return &ImportResult{
			ID:             mainID,
			SourcePath:     opts.originPath(),
			StoredPath:     existingEntry.StoredPath,
			MetadataPath:   existingEntry.MetadataPath,
			DetectedFormat: existingEntry.Format,
			DetectedOrigin: existingEntry.Origin,
			AssignedSchema: existingEntry.Schema,
			RecordCount:    existingEntry.RecordCount,
			SizeBytes:      existingEntry.SizeBytes,
			ContentHash:    contentHash,
			Skipped:        true,
			SkipReason:     "content already exists in catalog",
		}, nil
	}

	// Create filename using content hash (truncated for filesystem compatibility)
	ext := filepath.Ext(opts.SourcePath)
	filename := fmt.Sprintf("%s%s", mainID[:16], ext)
	storedPath, err := staged.Commit(filepath.Join(rawDir, filename))
	if err != nil {
		return nil, err
	}

	// Record collections stream: NDJSON, and a top-level JSON array. Both used to
	// be decoded whole into a slice before a single row was written, so peak
	// memory was the record set rather than one record.
	if collectionFormat, ok := streamableCollectionFormat(opts.SourcePath, format); ok {
		// Decoded from the staged copy, not the source: the bytes are identical
		// (that is what the hash asserts) and it is the copy that is warm in the
		// page cache, having just been written.
		opts.collectionFormat = format
		result, err := e.importCollectionStreamed(opts, mainID, timestamp, origin, storedPath,
			staged.Size, rawDir, metadataDir, collectionFormat)
		if err != nil {
			_ = os.Remove(storedPath)
			return nil, err
		}
		result.SourcePath = opts.originPath()
		return result, nil
	}

	result, err := e.importDocument(opts, documentImport{
		id:          mainID,
		contentHash: contentHash,
		format:      format,
		origin:      origin,
		timestamp:   timestamp,
		storedPath:  storedPath,
		metadataDir: metadataDir,
		sizeBytes:   staged.Size,
	})
	if err != nil {
		// The raw file is committed; a failure must not leave it behind as
		// evidence with no catalog row.
		_ = os.Remove(storedPath)
		return nil, err
	}
	return result, nil
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

// importCollectionStreamed imports a record collection one record at a time.
func (e *EnhancedImporter) importCollectionStreamed(opts ImportOptions, collectionID string, timestamp time.Time, origin, storedPath string, sizeBytes int64, rawDir, metadataDir, format string) (*ImportResult, error) {
	stream := &collectionStream{
		importer:     e,
		opts:         opts,
		collectionID: collectionID,
		timestamp:    timestamp,
		rawDir:       rawDir,
		metadataDir:  metadataDir,
	}
	return stream.run(storedPath, format, origin, storedPath, sizeBytes)
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
	if err := e.saveMetadata(*metadata, metadataPath); err != nil {
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
