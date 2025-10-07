package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/idgen"
)

// EnhancedImporter extends the base importer with human-friendly ID generation
type EnhancedImporter struct {
	*Importer // Embed the original importer
	
	configManager *config.ConfigManager
	idManager     *idgen.ImporterIDManager
	displayHelper *idgen.IDDisplayHelper
}

// NewEnhancedImporter creates a new enhanced importer with friendly ID support
func NewEnhancedImporter(dataPath, schemaPath, configDir string, catalogDB *database.CatalogDB) (*EnhancedImporter, error) {
	// Create base importer
	baseImporter, err := NewImporter(dataPath, schemaPath, catalogDB)
	if err != nil {
		return nil, fmt.Errorf("failed to create base importer: %w", err)
	}
	
	// Create config manager
	configManager := config.NewConfigManager(configDir)
	
	// Load configuration
	idConfig, err := configManager.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load ID configuration: %w", err)
	}
	
	// Create ID manager with default config
	defaultIDConfig := idConfig.GetConfigForOrigin("default")
	idManager := idgen.NewImporterIDManager(defaultIDConfig)
	
	// Create display helper
	displayHelper := idgen.NewIDDisplayHelper()
	
	return &EnhancedImporter{
		Importer:      baseImporter,
		configManager: configManager,
		idManager:     idManager,
		displayHelper: displayHelper,
	}, nil
}

// ImportFileWithFriendlyIDs imports a file using the new ID generation system
func (e *EnhancedImporter) ImportFileWithFriendlyIDs(opts ImportOptions) (*ImportResult, error) {
	// Load current configuration
	idConfig, err := e.configManager.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load ID configuration: %w", err)
	}
	
	// Check if friendly IDs are enabled
	if !idConfig.ShouldUseFriendlyIDs() {
		// Fall back to legacy import
		return e.Importer.ImportFile(opts)
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
	
	// Get ID configuration for this origin
	originConfig := idConfig.GetConfigForOrigin(origin)
	
	// Create ID manager for this specific import
	idManager := idgen.NewImporterIDManager(originConfig)
	
	// Perform import with friendly IDs
	return e.importWithFriendlyIDs(opts, idManager, origin)
}

// importWithFriendlyIDs performs the actual import using friendly ID generation
func (e *EnhancedImporter) importWithFriendlyIDs(opts ImportOptions, idManager *idgen.ImporterIDManager, origin string) (*ImportResult, error) {
	// Ensure basic schemas exist
	if err := e.ensureBasicSchemas(); err != nil {
		return nil, fmt.Errorf("failed to ensure basic schemas: %w", err)
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

	// Generate friendly ID for main entry
	mainID := idManager.GenerateMainID(opts.SourcePath, origin)
	
	// Create filename using friendly ID
	ext := filepath.Ext(opts.SourcePath)
	filename := fmt.Sprintf("%s%s", mainID, ext)

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
		return e.importNDJSONCollectionWithFriendlyIDs(opts, idManager, timestamp, origin, filename, rawDir, metadataDir, fileInfo)
	}

	// Read and analyze data for schema assignment
	var data interface{}
	var recordCount int

	if opts.UseStreaming {
		data, recordCount, err = e.analyzeDataStreaming(opts.SourcePath, format, opts.StreamingConfig)
	} else {
		data, recordCount, err = e.analyzeData(opts.SourcePath, format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to analyze data: %w", err)
	}

	// Copy file to storage
	storedPath := filepath.Join(rawDir, filename)
	if err := e.copyFile(opts.SourcePath, storedPath); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Assign schema
	schema, confidence := e.assignSchema(data, opts)

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

// importNDJSONCollectionWithFriendlyIDs handles NDJSON collections with friendly IDs
func (e *EnhancedImporter) importNDJSONCollectionWithFriendlyIDs(opts ImportOptions, idManager *idgen.ImporterIDManager, timestamp time.Time, origin, filename string, rawDir, metadataDir string, fileInfo os.FileInfo) (*ImportResult, error) {
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

	// Generate collection ID
	collectionID := idManager.GenerateCollectionID(opts.SourcePath, origin)

	// Create collection entry with friendly ID
	collectionResult, err := e.createCollectionEntryWithFriendlyIDs(opts, idManager, timestamp, origin, collectionID, storedPath, metadataDir, fileInfo, recordCount, data)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection entry: %w", err)
	}

	// Create individual item entries with friendly IDs
	if err := e.createCollectionItemsWithFriendlyIDs(collectionID, data, idManager, timestamp, rawDir, metadataDir, opts); err != nil {
		return nil, fmt.Errorf("failed to create collection items: %w", err)
	}

	return collectionResult, nil
}

// GetIDDisplayFormat returns a user-friendly display format for an ID
func (e *EnhancedImporter) GetIDDisplayFormat(id string) string {
	return e.displayHelper.FormatForDisplay(id)
}

// GetIDType returns the type of an ID (legacy, short, readable, etc.)
func (e *EnhancedImporter) GetIDType(id string) string {
	return e.displayHelper.GetIDType(id)
}

// UpdateIDConfiguration updates the ID generation configuration
func (e *EnhancedImporter) UpdateIDConfiguration(config *config.IDGenerationConfig) error {
	return e.configManager.UpdateConfig(config)
}

// GetIDConfiguration returns the current ID generation configuration
func (e *EnhancedImporter) GetIDConfiguration() *config.IDGenerationConfig {
	return e.configManager.GetConfig()
}
