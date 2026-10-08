package importer

import (
	"context"
	"fmt"
	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/ingestprep"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/validator"
)

// ImportOptions contains options for importing data
type ImportOptions struct {
	Context        context.Context
	Limits         ingestprep.Limits
	Explain        bool // Include classification diagnostics in the result.
	SourcePath     string
	Origin         string                    // Optional origin override
	ManualSchema   string                    // Manual schema specification
	ChainValidator *validator.ChainValidator // Validator for manual schema; built on demand when nil

	// OriginPath is the path origin and stored-file naming are derived from when
	// SourcePath is an intermediate (an envelope payload or a decompressed copy).
	// Empty means SourcePath.
	OriginPath string

	// Set assigns fields on every imported record (`pudl import --set`).
	Set []FieldAssignment
	// DryRun previews the import (see Preview) instead of performing it.
	DryRun bool

	// collectionFormat is the detected format of a streamed collection, set by
	// the importer itself so the collection entry records what was imported.
	collectionFormat string
	publication      *artifacts.Journal
	// assignments replays the pre-pass's per-record schema assignments.
	assignments *assignmentReader
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
	Explanation           *inference.InferenceTrace   `json:"explanation,omitempty"`
	ItemExplanations      []ItemExplanation           `json:"item_explanations,omitempty"`
	ExplanationsTruncated bool                        `json:"explanations_truncated,omitempty"`
	ID                    string                      `json:"id"`
	SourcePath            string                      `json:"source_path"`
	StoredPath            string                      `json:"stored_path"`
	MetadataPath          string                      `json:"metadata_path"`
	DetectedFormat        string                      `json:"detected_format"`
	DetectedOrigin        string                      `json:"detected_origin"`
	AssignedSchema        string                      `json:"assigned_schema"`
	SchemaConfidence      float64                     `json:"schema_confidence"`
	RecordCount           int                         `json:"record_count"`
	SizeBytes             int64                       `json:"size_bytes"`
	ImportTimestamp       string                      `json:"import_timestamp"`
	ValidationResult      *validator.ValidationResult `json:"validation_result,omitempty"`
	Skipped               bool                        `json:"skipped,omitempty"`
	SkipReason            string                      `json:"skip_reason,omitempty"`
	ResourceID            string                      `json:"resource_id,omitempty"`
	ContentHash           string                      `json:"content_hash,omitempty"`
	Version               int                         `json:"version,omitempty"`
	IsNewVersion          bool                        `json:"is_new_version,omitempty"`
	// IdentityUnresolved counts records whose schema declares identity fields
	// that could not be extracted; such records are identified by content hash
	// and do not join a version chain. IdentityError is the first failure.
	IdentityUnresolved int    `json:"identity_unresolved,omitempty"`
	IdentityError      string `json:"identity_error,omitempty"`
	// Redacted counts sensitive values replaced before storage.
	Redacted int `json:"redacted,omitempty"`
	// Reassigned counts already-cataloged records moved to the explicit
	// --schema because they now validate against it.
	Reassigned int `json:"reassigned,omitempty"`
	// Facts counts projected facts per relation (schemas with `_pudl.facts`);
	// FactWarnings says what was not projected and why.
	Facts        map[string]int `json:"facts,omitempty"`
	FactWarnings []string       `json:"fact_warnings,omitempty"`
	// DryRun marks a Preview result; Preview holds what it found.
	DryRun  bool           `json:"dry_run,omitempty"`
	Preview *PreviewReport `json:"preview,omitempty"`
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
	imp.redactor = redact.NewRegistry(inferrer)
	imp.projector = projection.NewRegistry(inferrer, imp.redactor)

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

// ItemExplanation is a bounded collection item classification diagnostic.
type ItemExplanation struct {
	Index int                       `json:"index"`
	Trace *inference.InferenceTrace `json:"trace"`
}
