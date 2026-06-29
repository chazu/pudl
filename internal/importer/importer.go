package importer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/streaming"
	"github.com/chazu/pudl/internal/validator"
)

// Importer handles data import operations
type Importer struct {
	dataPath     string
	schemaPath   string   // primary schema path (first in schemaPaths)
	schemaPaths  []string // all schema paths in priority order
	catalogDB    *database.CatalogDB
	inferrer     *inference.SchemaInferrer
}

// ImportOptions contains options for importing data
type ImportOptions struct {
	SourcePath       string
	Origin           string                      // Optional origin override
	ManualSchema     string                      // Manual schema specification
	ChainValidator *validator.ChainValidator // Validator for manual schema
	UseStreaming     bool                        // Whether to use streaming parser
	StreamingConfig  *streaming.StreamingConfig  // Configuration for streaming parser
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

// NewWithSchemaPaths creates a new Importer with multiple schema search paths.
// Paths are searched in order; earlier paths take priority (per-repo shadows global).
func NewWithSchemaPaths(dataPath, pudlHome string, schemaPaths ...string) (*Importer, error) {
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
	imp := &Importer{
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
	inferrer, err := inference.NewSchemaInferrer(schemaPaths...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize schema inferrer: %w", err)
	}
	imp.inferrer = inferrer

	return imp, nil
}

// Close closes the importer and its database connections
func (i *Importer) Close() error {
	// Close catalog database
	if i.catalogDB != nil {
		return i.catalogDB.Close()
	}
	return nil
}

// copyFile copies a file from src to dst
func (i *Importer) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
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

// analyzeDataDirect analyzes small structured files directly without streaming
func (i *Importer) analyzeDataDirect(filePath, format string) (interface{}, int, error) {
	// Read the entire file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read file: %w", err)
	}

	// Check for empty file
	if len(data) == 0 {
		if format == "json" {
			return nil, 0, fmt.Errorf("failed to parse JSON data: empty file (EOF)")
		}
		if format == "yaml" {
			return nil, 0, fmt.Errorf("failed to parse YAML data: empty file (EOF)")
		}
		return nil, 0, fmt.Errorf("empty file")
	}

	// Create processor registry and get the best processor
	registry := streaming.NewProcessorRegistry()
	processor := registry.GetBestProcessor(data)

	// Create a single chunk with all the data
	chunk := &streaming.CDCChunk{
		Data:     data,
		Offset:   0,
		Size:     len(data),
		Hash:     fmt.Sprintf("%x", sha256.Sum256(data)),
		Sequence: 0,
		Time:     time.Now(),
	}

	// Process the chunk
	result, err := processor.ProcessChunk(chunk)
	if err != nil {
		if format == "json" {
			return nil, 0, fmt.Errorf("failed to parse JSON data: %w", err)
		}
		if format == "yaml" {
			return nil, 0, fmt.Errorf("failed to parse YAML data: %w", err)
		}
		return nil, 0, fmt.Errorf("failed to parse %s data: %w", format, err)
	}

	// Check if any objects were extracted
	if len(result.Objects) == 0 {
		if format == "json" {
			return nil, 0, fmt.Errorf("failed to parse JSON data")
		}
		if format == "yaml" {
			return nil, 0, fmt.Errorf("failed to parse YAML data")
		}
		return nil, 0, fmt.Errorf("failed to parse %s data", format)
	}

	// Return the first object if there's only one, otherwise return all objects
	if len(result.Objects) == 1 {
		return result.Objects[0], len(result.Objects), nil
	}
	return result.Objects, len(result.Objects), nil
}

// analyzeDataStreaming analyzes data using the streaming parser for large files
func (i *Importer) analyzeDataStreaming(filePath, format string, config *streaming.StreamingConfig) (interface{}, int, error) {
	// Use default config if none provided
	if config == nil {
		config = streaming.DefaultStreamingConfig()
	}

	// Check file size and adjust chunk sizes for small files
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get file info: %w", err)
	}

	fileSize := fileInfo.Size()

	// For small structured files (< 10KB), bypass streaming and process directly
	// This avoids issues with CDC chunking splitting JSON/YAML objects
	if fileSize < 10*1024 && (format == "json" || format == "yaml") {
		return i.analyzeDataDirect(filePath, format)
	}

	// For larger files, use streaming with appropriate chunk sizes
	if fileSize < 10*1024 {
		// Small file: use tiny chunks to ensure proper chunking
		config.MinChunkSize = 64   // 64 bytes minimum
		config.AvgChunkSize = 256  // 256 bytes average
		config.MaxChunkSize = 1024 // 1KB maximum
	}

	// Create streaming parser
	parser, err := streaming.NewStreamingParser(config)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create streaming parser: %w", err)
	}
	defer parser.Close()

	// Set up progress reporter for debugging
	reporter := streaming.NewCLIProgressReporter(true) // Verbose mode
	parser.SetProgressReporter(reporter)

	// Set up schema detector (pass nil for now, would be schema manager in production)
	err = parser.SetCUESchemaDetector(nil)
	if err != nil {
		// Log warning but continue - schema detection is optional
		fmt.Printf("Warning: Failed to set schema detector: %v\n", err)
	}

	// Open file for streaming
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// File is ready for streaming

	// Create context (use background context like the demo)
	ctx := context.Background()

	// Parse the file using streaming
	chunks, errors := parser.Parse(ctx, file)

	// Collect all objects and count records
	var allObjects []interface{}
	var allErrors []error
	recordCount := 0

	// Process chunks as they arrive (using demo's approach)
	done := false
	for !done {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				done = true
				break
			}

			// Process chunk data

			// Add objects from this chunk
			allObjects = append(allObjects, chunk.Objects...)
			allErrors = append(allErrors, chunk.Errors...)
			recordCount += len(chunk.Objects)

		case err, ok := <-errors:
			if !ok {
				// Error channel closed
				continue
			}

			// Log error but continue processing (error tolerance)
			fmt.Printf("Warning: streaming parser error: %v\n", err)
		}
	}

	// Streaming parse complete

	// Handle error cases that should fail
	if len(allObjects) == 0 {
		// Check if this was supposed to be a structured format but failed to parse
		if format == "json" {
			// Check if file is empty
			if fileInfo, err := os.Stat(filePath); err == nil && fileInfo.Size() == 0 {
				return nil, 0, fmt.Errorf("failed to parse JSON data: empty file (EOF)")
			}
			// For JSON format, 0 objects usually means parsing failed
			return nil, 0, fmt.Errorf("failed to parse JSON data")
		}
		if format == "yaml" {
			// Check if file is empty
			if fileInfo, err := os.Stat(filePath); err == nil && fileInfo.Size() == 0 {
				return nil, 0, fmt.Errorf("failed to parse YAML data: empty file (EOF)")
			}
			// For YAML format, 0 objects usually means parsing failed
			return nil, 0, fmt.Errorf("failed to parse YAML data")
		}
		if format == "unknown" {
			// For unknown format, return a generic object
			return map[string]interface{}{"format": format}, 1, nil
		}
		return map[string]interface{}{"format": format}, 0, nil
	}

	// Check for invalid structured data that was treated as text or has errors
	if len(allObjects) == 1 {
		if obj, ok := allObjects[0].(map[string]interface{}); ok {
			// Check for raw_data (invalid structured data)
			if rawData, hasRaw := obj["raw_data"]; hasRaw {
				if (format == "json" || format == "yaml") && rawData != nil {
					// This was supposed to be structured data but was treated as raw data
					return nil, 0, fmt.Errorf("failed to parse %s format", format)
				}
			}

			// Check for text content when we expected structured data (YAML/JSON parsing failed)
			if content, hasContent := obj["content"]; hasContent {
				if (format == "json" || format == "yaml") && content != nil {
					// Check if there were parsing errors
					if len(allErrors) > 0 {
						return nil, 0, fmt.Errorf("failed to parse %s format: %v", format, allErrors[0])
					}
				}
			}
		}
	}

	// Handle return format based on data type and count
	if len(allObjects) == 1 {
		return allObjects[0], recordCount, nil
	} else {
		// For CSV format, convert to []map[string]string
		if format == "csv" {
			csvArray := make([]map[string]string, len(allObjects))
			for i, obj := range allObjects {
				if objMap, ok := obj.(map[string]interface{}); ok {
					strMap := make(map[string]string)
					for k, v := range objMap {
						// Skip internal metadata fields
						if k == "_column_count" || k == "_row_number" {
							continue
						}
						strMap[k] = fmt.Sprintf("%v", v)
					}
					csvArray[i] = strMap
				}
			}
			return csvArray, recordCount, nil
		}

		// For other formats, return as array
		return allObjects, recordCount, nil
	}
}

// assignItemSchema assigns a schema to an individual collection item using inference
func (i *Importer) assignItemSchema(itemData interface{}, opts ImportOptions) (string, float64) {
	// If manual schema is specified, use it
	if opts.ManualSchema != "" {
		return opts.ManualSchema, 0.9
	}

	// Use schema inferrer for automatic schema assignment
	result, err := i.inferrer.Infer(itemData, inference.InferenceHints{
		Format:         "json",
		CollectionType: "item",
	})
	if err != nil {
		// Fall back to Item schema on error
		return "pudl.schemas/pudl/core:#Item", 0.5
	}

	return result.Schema, result.Confidence
}
