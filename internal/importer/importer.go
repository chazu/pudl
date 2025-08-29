package importer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pudl/internal/validator"
)

// Importer handles data import operations
type Importer struct {
	dataPath   string
	schemaPath string
}

// ImportOptions contains options for importing data
type ImportOptions struct {
	SourcePath        string
	Origin           string                      // Optional origin override
	ManualSchema     string                      // Manual schema specification
	CascadeValidator *validator.CascadeValidator // Validator for manual schema
}

// ImportResult contains the results of an import operation
type ImportResult struct {
	ID               string                     `json:"id"`
	SourcePath       string                     `json:"source_path"`
	StoredPath       string                     `json:"stored_path"`
	MetadataPath     string                     `json:"metadata_path"`
	DetectedFormat   string                     `json:"detected_format"`
	DetectedOrigin   string                     `json:"detected_origin"`
	AssignedSchema   string                     `json:"assigned_schema"`
	SchemaConfidence float64                    `json:"schema_confidence"`
	RecordCount      int                        `json:"record_count"`
	SizeBytes        int64                      `json:"size_bytes"`
	ImportTimestamp  string                     `json:"import_timestamp"`
	ValidationResult *validator.ValidationResult `json:"validation_result,omitempty"`
}

// New creates a new Importer instance
func New(dataPath, schemaPath string) *Importer {
	return &Importer{
		dataPath:   dataPath,
		schemaPath: schemaPath,
	}
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

	// Copy file to raw storage
	storedPath := filepath.Join(rawDir, filename)
	if err := i.copyFile(opts.SourcePath, storedPath); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Read and analyze data for schema assignment
	data, recordCount, err := i.analyzeData(storedPath, format)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze data: %w", err)
	}

	// Assign schema using cascading validation or rule engine
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
		// Use legacy rule-based assignment
		schema, confidence = i.assignSchema(data, origin, format)
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
