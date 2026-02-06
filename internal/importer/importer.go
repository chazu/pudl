package importer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pudl/internal/database"
	"pudl/internal/inference"
	"pudl/internal/streaming"
	"pudl/internal/validator"
)

// Importer handles data import operations
type Importer struct {
	dataPath   string
	schemaPath string
	catalogDB  *database.CatalogDB
	inferrer   *inference.SchemaInferrer
}

// ImportOptions contains options for importing data
type ImportOptions struct {
	SourcePath       string
	Origin           string                      // Optional origin override
	ManualSchema     string                      // Manual schema specification
	CascadeValidator *validator.CascadeValidator // Validator for manual schema
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

// New creates a new Importer instance
func New(dataPath, schemaPath, pudlHome string) (*Importer, error) {
	// Validate schema path is provided
	if schemaPath == "" {
		return nil, fmt.Errorf("schema path is required")
	}

	// Initialize catalog database
	catalogDB, err := database.NewCatalogDB(pudlHome)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize catalog database: %w", err)
	}

	// Create importer first (without inferrer)
	imp := &Importer{
		dataPath:   dataPath,
		schemaPath: schemaPath,
		catalogDB:  catalogDB,
	}

	// Ensure bootstrap schemas exist before loading the inferrer
	if err := imp.ensureBasicSchemas(); err != nil {
		return nil, fmt.Errorf("failed to ensure basic schemas: %w", err)
	}

	// Initialize schema inferrer (now schemas should exist)
	inferrer, err := inference.NewSchemaInferrer(schemaPath)
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

// GetAvailableSchemas returns the names of all schemas available for inference
func (i *Importer) GetAvailableSchemas() []string {
	if i.inferrer == nil {
		return nil
	}
	return i.inferrer.GetAvailableSchemas()
}

// ReloadSchemas reloads schemas from the schema repository
func (i *Importer) ReloadSchemas() error {
	if i.inferrer == nil {
		return fmt.Errorf("schema inferrer not initialized")
	}
	return i.inferrer.Reload()
}

// ImportFile imports a single file into the data lake
func (i *Importer) ImportFile(opts ImportOptions) (*ImportResult, error) {
	// Ensure basic schemas exist
	if err := i.ensureBasicSchemas(); err != nil {
		return nil, fmt.Errorf("failed to ensure basic schemas: %w", err)
	}

	// Get file info
	fileInfo, err := os.Stat(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Generate timestamp for naming
	timestamp := time.Now()
	timestampStr := timestamp.Format("20060102_150405")

	// Detect format
	format, err := i.detectFormat(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect format: %w", err)
	}

	// Detect or use provided origin
	origin := opts.Origin
	if origin == "" {
		origin = i.detectOrigin(opts.SourcePath, format)
	}

	// Create filename for storage
	ext := filepath.Ext(opts.SourcePath)
	filename := fmt.Sprintf("%s_%s%s", timestampStr, origin, ext)

	// Create date-based directory structure
	dateDir := timestamp.Format("2006/01/02")
	rawDir := filepath.Join(i.dataPath, "raw", dateDir)
	metadataDir := filepath.Join(i.dataPath, "metadata")

	// Ensure directories exist
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create raw directory: %w", err)
	}
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Handle NDJSON collections differently
	if format == "ndjson" {
		return i.importNDJSONCollection(opts, timestamp, timestampStr, origin, filename, rawDir, metadataDir, fileInfo)
	}

	// Read and analyze data for schema assignment BEFORE copying
	// This ensures we can read the original file without any file handle conflicts
	var data interface{}
	var recordCount int

	// Detect compression and decompress if needed for analysis
	compression := DetectCompression(opts.SourcePath)
	fileToAnalyze := opts.SourcePath
	if compression != "none" {
		decompressed, err := DecompressFile(opts.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress file: %w", err)
		}
		fileToAnalyze = decompressed
		defer os.Remove(decompressed) // Clean up temporary decompressed file
	}

	// Always use streaming parser for optimal performance and memory usage
	data, recordCount, err = i.analyzeDataStreaming(fileToAnalyze, format, opts.StreamingConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze data: %w", err)
	}

	// Copy file to raw storage AFTER analysis
	storedPath := filepath.Join(rawDir, filename)
	if err := i.copyFile(opts.SourcePath, storedPath); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Assign schema using cascading validation or inference
	var schema string
	var confidence float64
	var validationResult *validator.ValidationResult

	if opts.ManualSchema != "" && opts.CascadeValidator != nil {
		// Use cascading validation for manual schema specification
		vr, err := opts.CascadeValidator.ValidateWithCascade(data, opts.ManualSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to perform cascading validation: %w", err)
		}
		validationResult = vr
		schema = vr.AssignedSchema
		confidence = 1.0 // High confidence for validated data
	} else {
		// Use schema inferrer for automatic schema assignment
		result, err := i.inferrer.Infer(data, inference.InferenceHints{
			Origin: origin,
			Format: format,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to infer schema: %w", err)
		}
		schema = result.Schema
		confidence = result.Confidence
	}

	// Create metadata
	metadata := ImportMetadata{
		ID: strings.TrimSuffix(filename, ext),
		SourceInfo: SourceInfo{
			Origin:       origin,
			OriginalPath: opts.SourcePath,
			Confidence:   "high", // Simple confidence for now
		},
		ImportMetadata: ImportMeta{
			Timestamp:   timestamp.Format(time.RFC3339),
			Format:      format,
			SizeBytes:   fileInfo.Size(),
			RecordCount: recordCount,
		},
		SchemaInfo: SchemaInfo{
			CuePackage:       extractPackage(schema),
			CueDefinition:    schema,
			SchemaFile:       "", // Will be populated when we have actual CUE files
			SchemaVersion:    "v1.0",
			ValidationStatus: getValidationStatus(validationResult),
			IntendedSchema:   getIntendedSchema(validationResult),
			ComplianceStatus: getComplianceStatus(validationResult),
			CascadeLevel:     getCascadeLevel(validationResult),
		},
		ResourceTracking: ResourceTracking{
			IdentityFields: []string{}, // Will be extracted from CUE schema later
			TrackedFields:  []string{}, // Will be extracted from CUE schema later
		},
	}

	// Save metadata
	metadataPath := filepath.Join(metadataDir, filename+".meta")
	if err := i.saveMetadata(metadata, metadataPath); err != nil {
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	// Update catalog (inventory)
	if err := i.updateCatalog(metadata, storedPath, metadataPath); err != nil {
		return nil, fmt.Errorf("failed to update catalog: %w", err)
	}

	// Return result
	return &ImportResult{
		ID:               strings.TrimSuffix(filename, ext),
		SourcePath:       opts.SourcePath,
		StoredPath:       storedPath,
		MetadataPath:     metadataPath,
		DetectedFormat:   format,
		DetectedOrigin:   origin,
		AssignedSchema:   schema,
		SchemaConfidence: confidence,
		RecordCount:      recordCount,
		SizeBytes:        fileInfo.Size(),
		ImportTimestamp:  timestamp.Format(time.RFC3339),
		ValidationResult: validationResult,
	}, nil
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

// Helper functions for validation result processing

func getValidationStatus(vr *validator.ValidationResult) string {
	if vr == nil {
		return "auto-assigned"
	}
	if vr.Success {
		return "validated"
	}
	return "failed"
}

func getIntendedSchema(vr *validator.ValidationResult) string {
	if vr == nil {
		return ""
	}
	return vr.IntendedSchema
}

func getComplianceStatus(vr *validator.ValidationResult) string {
	if vr == nil {
		return "unknown"
	}
	return vr.GetComplianceStatus()
}

func getCascadeLevel(vr *validator.ValidationResult) string {
	if vr == nil {
		return "auto"
	}
	return vr.CascadeLevel
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

// importNDJSONCollection handles importing NDJSON files as collections with individual items
func (i *Importer) importNDJSONCollection(opts ImportOptions, timestamp time.Time, timestampStr, origin, filename string, rawDir, metadataDir string, fileInfo os.FileInfo) (*ImportResult, error) {
	// Detect compression and decompress if needed for analysis
	compression := DetectCompression(opts.SourcePath)
	fileToAnalyze := opts.SourcePath
	if compression != "none" {
		decompressed, err := DecompressFile(opts.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress file: %w", err)
		}
		fileToAnalyze = decompressed
		defer os.Remove(decompressed) // Clean up temporary decompressed file
	}

	// Parse NDJSON file using streaming to get individual objects
	data, recordCount, err := i.analyzeDataStreaming(fileToAnalyze, "json", opts.StreamingConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze NDJSON data: %w", err)
	}

	// Copy original file to raw storage
	storedPath := filepath.Join(rawDir, filename)
	if err := i.copyFile(opts.SourcePath, storedPath); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Generate collection ID (same as main entry ID)
	collectionID := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Create collection entry
	collectionResult, err := i.createCollectionEntry(opts, timestamp, timestampStr, origin, filename, storedPath, metadataDir, fileInfo, recordCount, data)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection entry: %w", err)
	}

	// Create individual item entries
	if err := i.createCollectionItems(collectionID, data, timestamp, rawDir, metadataDir, opts); err != nil {
		return nil, fmt.Errorf("failed to create collection items: %w", err)
	}

	return collectionResult, nil
}

// createCollectionEntry creates the main collection catalog entry
func (i *Importer) createCollectionEntry(opts ImportOptions, timestamp time.Time, timestampStr, origin, filename, storedPath, metadataDir string, fileInfo os.FileInfo, recordCount int, data interface{}) (*ImportResult, error) {
	// Assign schema for collection - try collection-specific schemas first
	schema := "pudl.schemas/pudl/core:#Collection"
	confidence := 0.8

	// All collections use the generic collection schema now
	// Content-specific metadata can be stored in the flexible collection_metadata field

	// Create metadata for collection
	metadata := &ImportMetadata{
		ID: strings.TrimSuffix(filename, filepath.Ext(filename)),
		SourceInfo: SourceInfo{
			OriginalPath: opts.SourcePath,
			Origin:       origin,
			Confidence:   "high",
		},
		ImportMetadata: ImportMeta{
			Format:      "ndjson",
			RecordCount: recordCount,
			SizeBytes:   fileInfo.Size(),
			Timestamp:   timestamp.Format(time.RFC3339),
		},
		SchemaInfo: SchemaInfo{
			CuePackage:       extractPackage(schema),
			CueDefinition:    schema,
			ValidationStatus: "auto-assigned",
			CascadeLevel:     "auto",
			ComplianceStatus: "unknown",
			SchemaVersion:    "v1.0",
		},
		ResourceTracking: ResourceTracking{
			IdentityFields: []string{"collection_id"},
			TrackedFields:  []string{"item_count", "item_schemas"},
		},
	}

	// Save metadata
	metadataPath := filepath.Join(metadataDir, filename+".meta")
	if err := i.saveMetadata(*metadata, metadataPath); err != nil {
		return nil, fmt.Errorf("failed to save collection metadata: %w", err)
	}

	// Create collection catalog entry
	collectionType := "collection"
	entry := database.CatalogEntry{
		ID:              metadata.ID,
		StoredPath:      storedPath,
		MetadataPath:    metadataPath,
		ImportTimestamp: timestamp,
		Format:          "ndjson",
		Origin:          origin,
		Schema:          schema,
		Confidence:      confidence,
		RecordCount:     recordCount,
		SizeBytes:       fileInfo.Size(),
		CollectionID:    nil, // Collections don't have parent collections
		ItemIndex:       nil, // Collections don't have item index
		CollectionType:  &collectionType,
		ItemID:          nil, // Collections don't have item IDs
	}

	// Add to database
	if err := i.catalogDB.AddEntry(entry); err != nil {
		return nil, fmt.Errorf("failed to add collection to catalog: %w", err)
	}

	// Return result
	return &ImportResult{
		ID:               metadata.ID,
		SourcePath:       opts.SourcePath,
		StoredPath:       storedPath,
		MetadataPath:     metadataPath,
		DetectedFormat:   "ndjson",
		DetectedOrigin:   origin,
		AssignedSchema:   schema,
		SchemaConfidence: confidence,
		RecordCount:      recordCount,
		SizeBytes:        fileInfo.Size(),
		ImportTimestamp:  timestamp.Format(time.RFC3339),
		ValidationResult: nil, // Collections don't have individual validation results
	}, nil
}

// createCollectionItems creates individual catalog entries for each item in the collection
func (i *Importer) createCollectionItems(collectionID string, data interface{}, timestamp time.Time, rawDir, metadataDir string, opts ImportOptions) error {
	items, ok := data.([]interface{})
	if !ok {
		return fmt.Errorf("expected array of items for collection, got %T", data)
	}

	for index, item := range items {
		if err := i.createCollectionItem(collectionID, item, index, timestamp, rawDir, metadataDir, opts); err != nil {
			// Log error but continue with other items
			fmt.Printf("Warning: failed to create collection item %d: %v\n", index, err)
		}
	}

	return nil
}

// createCollectionItem creates a single collection item entry
func (i *Importer) createCollectionItem(collectionID string, itemData interface{}, index int, timestamp time.Time, rawDir, metadataDir string, opts ImportOptions) error {
	// Generate unique item ID
	itemID := fmt.Sprintf("%s_item_%d", collectionID, index)

	// Try to extract a more meaningful ID from the item data if possible
	if itemMap, ok := itemData.(map[string]interface{}); ok {
		if id, exists := itemMap["id"]; exists {
			if idStr, ok := id.(string); ok && idStr != "" {
				itemID = fmt.Sprintf("%s_%s", collectionID, idStr)
			}
		} else if externalID, exists := itemMap["externalId"]; exists {
			if extIDStr, ok := externalID.(string); ok && extIDStr != "" {
				// Use a hash of external ID to keep it manageable
				itemID = fmt.Sprintf("%s_%x", collectionID, fmt.Sprintf("%x", extIDStr)[:8])
			}
		}
	}

	// Assign schema to individual item
	schema, confidence := i.assignItemSchema(itemData, opts)

	// Create item data file
	itemFilename := fmt.Sprintf("%s.json", itemID)
	itemPath := filepath.Join(rawDir, itemFilename)

	// Save individual item as JSON file
	itemJSON, err := json.MarshalIndent(itemData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal item data: %w", err)
	}

	if err := os.WriteFile(itemPath, itemJSON, 0644); err != nil {
		return fmt.Errorf("failed to write item file: %w", err)
	}

	// Create item metadata
	itemMetadata := &ImportMetadata{
		ID: itemID,
		SourceInfo: SourceInfo{
			OriginalPath: opts.SourcePath,
			Origin:       fmt.Sprintf("%s_item_%d", collectionID, index),
			Confidence:   "high",
		},
		ImportMetadata: ImportMeta{
			Format:      "json",
			RecordCount: 1,
			SizeBytes:   int64(len(itemJSON)),
			Timestamp:   timestamp.Format(time.RFC3339),
		},
		SchemaInfo: SchemaInfo{
			CuePackage:       extractPackage(schema),
			CueDefinition:    schema,
			ValidationStatus: "auto-assigned",
			CascadeLevel:     "auto",
			ComplianceStatus: "unknown",
			SchemaVersion:    "v1.0",
		},
		ResourceTracking: ResourceTracking{
			IdentityFields: []string{"item_id"},
			TrackedFields:  []string{"item_data"},
		},
	}

	// Save item metadata
	itemMetadataPath := filepath.Join(metadataDir, itemFilename+".meta")
	if err := i.saveMetadata(*itemMetadata, itemMetadataPath); err != nil {
		return fmt.Errorf("failed to save item metadata: %w", err)
	}

	// Create catalog entry for item
	collectionType := "item"
	entry := database.CatalogEntry{
		ID:              itemID,
		StoredPath:      itemPath,
		MetadataPath:    itemMetadataPath,
		ImportTimestamp: timestamp,
		Format:          "json",
		Origin:          fmt.Sprintf("%s_item_%d", collectionID, index),
		Schema:          schema,
		Confidence:      confidence,
		RecordCount:     1,
		SizeBytes:       int64(len(itemJSON)),
		CollectionID:    &collectionID,
		ItemIndex:       &index,
		CollectionType:  &collectionType,
		ItemID:          &itemID,
	}

	// Add to database
	return i.catalogDB.AddEntry(entry)
}

// assignItemSchema assigns a schema to an individual collection item using inference
func (i *Importer) assignItemSchema(itemData interface{}, opts ImportOptions) (string, float64) {
	// If manual schema is specified, use it
	if opts.ManualSchema != "" {
		return opts.ManualSchema, 0.9
	}

	// Use schema inferrer for automatic schema assignment
	result, err := i.inferrer.Infer(itemData, inference.InferenceHints{
		Format: "json",
	})
	if err != nil {
		// Fall back to Item schema on error
		return "pudl.schemas/pudl/core:#Item", 0.5
	}

	return result.Schema, result.Confidence
}
