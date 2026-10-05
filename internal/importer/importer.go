package importer

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/validator"
)

// ImportOptions contains options for importing data
type ImportOptions struct {
	SourcePath     string
	Origin         string                    // Optional origin override
	ManualSchema   string                    // Manual schema specification
	ChainValidator *validator.ChainValidator // Validator for manual schema; built on demand when nil

	// OriginPath is the path origin and stored-file naming are derived from when
	// SourcePath is an intermediate (an envelope payload or a decompressed copy).
	// Empty means SourcePath.
	OriginPath string

	// collectionFormat is the detected format of a streamed collection, set by
	// the importer itself so the collection entry records what was imported.
	collectionFormat string
}

// originPath returns the path that names this import for origin detection.
func (o ImportOptions) originPath() string {
	if o.OriginPath != "" {
		return o.OriginPath
	}
	return o.SourcePath
}

// ImportResult contains the results of an import operation
type ImportResult struct {
	ID               string                      `json:"id"`
	SourcePath       string                      `json:"source_path"`
	StoredPath       string                      `json:"stored_path"`
	MetadataPath     string                      `json:"metadata_path"`
	DetectedFormat   string                      `json:"detected_format"`
	DetectedOrigin   string                      `json:"detected_origin"`
	AssignedSchema   string                      `json:"assigned_schema"`
	SchemaConfidence float64                     `json:"schema_confidence"`
	RecordCount      int                         `json:"record_count"`
	SizeBytes        int64                       `json:"size_bytes"`
	ImportTimestamp  string                      `json:"import_timestamp"`
	ValidationResult *validator.ValidationResult `json:"validation_result,omitempty"`
	Skipped          bool                        `json:"skipped,omitempty"`
	SkipReason       string                      `json:"skip_reason,omitempty"`
	ResourceID       string                      `json:"resource_id,omitempty"`
	ContentHash      string                      `json:"content_hash,omitempty"`
	Version          int                         `json:"version,omitempty"`
	IsNewVersion     bool                        `json:"is_new_version,omitempty"`
}

// newImporterState builds the importer's shared state from multiple schema
// search paths. Paths are searched in order; earlier paths take priority
// (per-repo shadows global).
//
// This used to construct a separate `Importer` type that `EnhancedImporter`
// embedded. Nothing outside this package ever built one — NewEnhancedImporter*
// is the only entry point — so the embed was pure layering, and it was the
// layering that made the memory-unbounded path easy to miss: the accumulating
// call sat in the embedded type while the pipeline above it read as if it
// streamed.
func newImporterState(dataPath, pudlHome string, schemaPaths ...string) (*EnhancedImporter, error) {
	if len(schemaPaths) == 0 {
		return nil, fmt.Errorf("at least one schema path is required")
	}

	// Use the first non-empty path as the primary schema path
	primarySchemaPath := ""
	for _, sp := range schemaPaths {
		if sp != "" {
			primarySchemaPath = sp
			break
		}
	}
	if primarySchemaPath == "" {
		return nil, fmt.Errorf("schema path is required")
	}

	// Initialize catalog database
	catalogDB, err := database.NewCatalogDB(pudlHome)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize catalog database: %w", err)
	}

	// Create importer first (without inferrer)
	imp := &EnhancedImporter{
		dataPath:    dataPath,
		schemaPath:  primarySchemaPath,
		schemaPaths: schemaPaths,
		catalogDB:   catalogDB,
	}

	// Ensure bootstrap schemas exist before loading the inferrer
	if err := imp.ensureBasicSchemas(); err != nil {
		return nil, fmt.Errorf("failed to ensure basic schemas: %w", err)
	}

	// Initialize schema inferrer with all paths
	inferrer, err := inference.Shared(schemaPaths...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize schema inferrer: %w", err)
	}
	imp.inferrer = inferrer

	return imp, nil
}

// Close closes the importer and its database connections
func (e *EnhancedImporter) Close() error {
	// Close catalog database
	if e.catalogDB != nil {
		return e.catalogDB.Close()
	}
	return nil
}

// extractPackage extracts the package name from a schema definition
func extractPackage(schema string) string {
	if strings.Contains(schema, ".") {
		parts := strings.Split(schema, ".")
		if len(parts) > 1 {
			return parts[0]
		}
	}
	return "unknown"
}

// analyzeData decodes a non-collection source for schema inference and
// identity extraction.
func (e *EnhancedImporter) analyzeData(filePath, format string) (interface{}, int, error) {
	return DecodeFile(filePath, format)
}
