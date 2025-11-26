package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pudl/internal/database"
	"pudl/internal/idgen"
	"pudl/internal/inference"
)

// EnhancedImporter extends the base importer with content-based ID generation
type EnhancedImporter struct {
	*Importer // Embed the original importer
}

// NewEnhancedImporter creates a new enhanced importer with content-based ID support
func NewEnhancedImporter(dataPath, schemaPath, configDir string) (*EnhancedImporter, error) {
	// Create base importer
	baseImporter, err := New(dataPath, schemaPath, configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create base importer: %w", err)
	}

	return &EnhancedImporter{
		Importer: baseImporter,
	}, nil
}

// ImportFileWithFriendlyIDs imports a file using content-based ID generation
func (e *EnhancedImporter) ImportFileWithFriendlyIDs(opts ImportOptions) (*ImportResult, error) {
	// Ensure basic schemas exist
	if err := e.ensureBasicSchemas(); err != nil {
		return nil, fmt.Errorf("failed to ensure basic schemas: %w", err)
	}

	// Detect origin if not provided
	origin := opts.Origin
	if origin == "" {
		format, err := e.detectFormat(opts.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("failed to detect format: %w", err)
		}
		origin = e.detectOrigin(opts.SourcePath, format)
	}

	// Get file info
	fileInfo, err := os.Stat(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Generate timestamp for metadata
	timestamp := time.Now()

	// Detect format
	format, err := e.detectFormat(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect format: %w", err)
	}

	// Read file data to compute content hash
	fileData, err := os.ReadFile(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Compute content-based ID (SHA256 hash)
	mainID := idgen.ComputeContentID(fileData)

	// Check if this content already exists in the catalog
	exists, err := e.catalogDB.EntryExists(mainID)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing entry: %w", err)
	}
	if exists {
		// Content already imported - return a result indicating it was skipped
		existingEntry, _ := e.catalogDB.GetEntry(mainID)
		return &ImportResult{
			ID:             mainID,
			SourcePath:     opts.SourcePath,
			StoredPath:     existingEntry.StoredPath,
			MetadataPath:   existingEntry.MetadataPath,
			DetectedFormat: existingEntry.Format,
			DetectedOrigin: existingEntry.Origin,
			AssignedSchema: existingEntry.Schema,
			RecordCount:    existingEntry.RecordCount,
			SizeBytes:      existingEntry.SizeBytes,
			Skipped:        true,
			SkipReason:     "content already exists in catalog",
		}, nil
	}

	// Create filename using content hash (truncated for filesystem compatibility)
	ext := filepath.Ext(opts.SourcePath)
	// Use first 16 chars of hash for filename (still unique, more manageable)
	filename := fmt.Sprintf("%s%s", mainID[:16], ext)

	// Create date-based directory structure (keep this for organization)
	dateDir := timestamp.Format("2006/01/02")
	rawDir := filepath.Join(e.dataPath, "raw", dateDir)
	metadataDir := filepath.Join(e.dataPath, "metadata")

	// Ensure directories exist
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create raw directory: %w", err)
	}
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Handle NDJSON collections differently
	if format == "ndjson" {
		return e.importNDJSONCollectionWithContentHash(opts, mainID, timestamp, origin, filename, rawDir, metadataDir, fileInfo, fileData)
	}

	// Analyze data for schema assignment (parse the data we already read)
	var data interface{}
	var recordCount int

	// Use streaming parser for optimal performance
	data, recordCount, err = e.analyzeDataStreaming(opts.SourcePath, format, opts.StreamingConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze data: %w", err)
	}

	// Copy file to storage
	storedPath := filepath.Join(rawDir, filename)
	if err := e.copyFile(opts.SourcePath, storedPath); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Assign schema using inference
	result, err := e.inferrer.Infer(data, inference.InferenceHints{
		Origin: origin,
		Format: format,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to infer schema: %w", err)
	}
	schema := result.Schema
	confidence := result.Confidence

	// Create metadata with friendly ID
	metadata := ImportMetadata{
		ID: mainID, // Use friendly ID instead of filename-based ID
		SourceInfo: SourceInfo{
			Origin:       origin,
			OriginalPath: opts.SourcePath,
			Confidence:   "high",
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
			ValidationStatus: "auto-assigned",
			CascadeLevel:     "auto",
			ComplianceStatus: "unknown",
			SchemaVersion:    "v1.0",
		},
		ResourceTracking: ResourceTracking{
			IdentityFields: []string{"id"},
			TrackedFields:  []string{"data"},
		},
	}

	// Save metadata
	metadataPath := filepath.Join(metadataDir, mainID+".meta")
	if err := e.saveMetadata(metadata, metadataPath); err != nil {
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	// Create catalog entry
	entry := database.CatalogEntry{
		ID:              mainID,
		StoredPath:      storedPath,
		MetadataPath:    metadataPath,
		ImportTimestamp: timestamp,
		Format:          format,
		Origin:          origin,
		Schema:          schema,
		Confidence:      confidence,
		RecordCount:     recordCount,
		SizeBytes:       fileInfo.Size(),
	}

	// Add to database
	if err := e.catalogDB.AddEntry(entry); err != nil {
		return nil, fmt.Errorf("failed to add to catalog: %w", err)
	}

	// Return result
	return &ImportResult{
		ID:               mainID,
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
		ValidationResult: nil,
	}, nil
}

// importNDJSONCollectionWithContentHash handles NDJSON collections with content-based IDs
func (e *EnhancedImporter) importNDJSONCollectionWithContentHash(opts ImportOptions, collectionID string, timestamp time.Time, origin, filename string, rawDir, metadataDir string, fileInfo os.FileInfo, fileData []byte) (*ImportResult, error) {
	// Parse NDJSON file
	data, recordCount, err := e.analyzeDataStreaming(opts.SourcePath, "json", opts.StreamingConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze NDJSON data: %w", err)
	}

	// Copy original file to raw storage
	storedPath := filepath.Join(rawDir, filename)
	if err := e.copyFile(opts.SourcePath, storedPath); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Create collection entry with content hash ID
	collectionResult, err := e.createCollectionEntryWithContentHash(opts, timestamp, origin, collectionID, storedPath, metadataDir, fileInfo, recordCount, data)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection entry: %w", err)
	}

	// Create individual item entries
	if err := e.createCollectionItemsWithContentHash(collectionID, data, timestamp, rawDir, metadataDir, opts); err != nil {
		return nil, fmt.Errorf("failed to create collection items: %w", err)
	}

	return collectionResult, nil
}

// GetIDDisplayFormat returns a proquint display format for a content hash ID
func (e *EnhancedImporter) GetIDDisplayFormat(id string) string {
	return idgen.HashToProquint(id)
}

// createCollectionEntryWithContentHash creates the main collection catalog entry with content hash IDs
func (e *EnhancedImporter) createCollectionEntryWithContentHash(opts ImportOptions, timestamp time.Time, origin, collectionID, storedPath, metadataDir string, fileInfo os.FileInfo, recordCount int, data interface{}) (*ImportResult, error) {
	// Assign schema for collection - try collection-specific schemas first
	schema := "pudl.schemas/collections/collections:#Collection"
	confidence := 0.8

	// Create metadata for collection
	metadata := &ImportMetadata{
		ID: collectionID,
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

	// Save collection metadata
	metadataPath := filepath.Join(metadataDir, collectionID+".meta")
	if err := e.saveMetadata(*metadata, metadataPath); err != nil {
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
	if err := e.catalogDB.AddEntry(entry); err != nil {
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

// createCollectionItemsWithContentHash creates individual catalog entries for each item in the collection with content hash IDs
func (e *EnhancedImporter) createCollectionItemsWithContentHash(collectionID string, data interface{}, timestamp time.Time, rawDir, metadataDir string, opts ImportOptions) error {
	items, ok := data.([]interface{})
	if !ok {
		return fmt.Errorf("expected array of items for collection, got %T", data)
	}

	for index, item := range items {
		if err := e.createCollectionItemWithContentHash(collectionID, item, index, timestamp, rawDir, metadataDir, opts); err != nil {
			// Log error but continue with other items
			fmt.Printf("Warning: failed to create collection item %d: %v\n", index, err)
		}
	}

	return nil
}

// createCollectionItemWithContentHash creates a single collection item entry with content hash IDs
func (e *EnhancedImporter) createCollectionItemWithContentHash(collectionID string, itemData interface{}, index int, timestamp time.Time, rawDir, metadataDir string, opts ImportOptions) error {
	// Generate item ID based on content hash of the item data
	itemJSON, err := json.Marshal(itemData)
	if err != nil {
		return fmt.Errorf("failed to marshal item data: %w", err)
	}
	itemID := idgen.ComputeContentID(itemJSON)

	// Create filename for individual item
	itemFilename := fmt.Sprintf("%s_item_%d", collectionID, index)
	itemPath := filepath.Join(rawDir, itemFilename+".json")

	// Save individual item as JSON (use indented format for storage)
	itemJSON, err = json.MarshalIndent(itemData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal item data: %w", err)
	}

	if err := os.WriteFile(itemPath, itemJSON, 0644); err != nil {
		return fmt.Errorf("failed to write item file: %w", err)
	}

	// Assign schema to item
	schema, confidence := e.assignItemSchema(itemData, opts)

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
	if err := e.saveMetadata(*itemMetadata, itemMetadataPath); err != nil {
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
	return e.catalogDB.AddEntry(entry)
}
