package streaming

import (
	"fmt"
	"path/filepath"
	"strings"

	"pudl/internal/schema"
)

// CUESchemaDetector integrates schema detection with PUDL's CUE schema system
type CUESchemaDetector struct {
	simpleDetector *SimpleSchemaDetector
	schemaManager  *schema.Manager
	schemaCache    map[string]*schema.SchemaInfo
}

// NewCUESchemaDetector creates a new CUE-integrated schema detector
func NewCUESchemaDetector(schemaManager *schema.Manager, maxSamples int) *CUESchemaDetector {
	return &CUESchemaDetector{
		simpleDetector: NewSimpleSchemaDetector(maxSamples),
		schemaManager:  schemaManager,
		schemaCache:    make(map[string]*schema.SchemaInfo),
	}
}

// AddSample adds a chunk sample for schema detection
func (d *CUESchemaDetector) AddSample(chunk *ProcessedChunk) error {
	return d.simpleDetector.AddSample(chunk)
}

// DetectSchema returns the detected schema with CUE integration
func (d *CUESchemaDetector) DetectSchema() (*SchemaDetection, error) {
	// Get detection from simple detector
	detection, err := d.simpleDetector.DetectSchema()
	if err != nil {
		return nil, err
	}
	
	if detection == nil || detection.SchemaName == "unknown" {
		return detection, nil
	}
	
	// Try to find corresponding CUE schema
	cueSchema, err := d.findCUESchema(detection.SchemaName)
	if err != nil {
		// Log error but don't fail - return detection without CUE integration
		detection.Metadata["cue_error"] = err.Error()
		return detection, nil
	}
	
	if cueSchema != nil {
		// Enhance detection with CUE schema information
		detection.Metadata["cue_schema"] = cueSchema.Name
		detection.Metadata["cue_package"] = cueSchema.Package
		detection.Metadata["cue_file"] = cueSchema.FilePath
		
		// Validate against CUE schema if possible
		if validationResult := d.validateAgainstCUE(detection, cueSchema); validationResult != nil {
			detection.Metadata["cue_validation"] = validationResult
		}
	}
	
	return detection, nil
}

// Reset clears all samples and cache
func (d *CUESchemaDetector) Reset() {
	d.simpleDetector.Reset()
	d.schemaCache = make(map[string]*schema.SchemaInfo)
}

// GetConfidence returns the current confidence level
func (d *CUESchemaDetector) GetConfidence() float64 {
	return d.simpleDetector.GetConfidence()
}

// findCUESchema attempts to find a CUE schema matching the detected schema name
func (d *CUESchemaDetector) findCUESchema(schemaName string) (*schema.SchemaInfo, error) {
	// Check cache first
	if cached, exists := d.schemaCache[schemaName]; exists {
		return cached, nil
	}

	// Try different schema name formats
	candidates := d.generateSchemaCandidates(schemaName)

	for _, candidate := range candidates {
		cueSchema, err := d.loadCUESchema(candidate)
		if err == nil && cueSchema != nil {
			// Cache the result
			d.schemaCache[schemaName] = cueSchema
			return cueSchema, nil
		}
	}

	return nil, fmt.Errorf("no CUE schema found for %s", schemaName)
}

// generateSchemaCandidates generates possible CUE schema names from detected schema name
func (d *CUESchemaDetector) generateSchemaCandidates(schemaName string) []string {
	candidates := []string{schemaName}
	
	// Convert dot notation to different formats
	if strings.Contains(schemaName, ".") {
		parts := strings.Split(schemaName, ".")
		
		// Try package.schema format
		if len(parts) == 2 {
			candidates = append(candidates, 
				fmt.Sprintf("%s.%s", parts[0], parts[1]),
				fmt.Sprintf("%s/%s", parts[0], parts[1]),
				parts[1], // Just the schema name
			)
		}
	}
	
	// Try common prefixes/suffixes
	baseName := strings.TrimPrefix(schemaName, "aws.")
	baseName = strings.TrimPrefix(baseName, "k8s.")
	baseName = strings.TrimPrefix(baseName, "kubernetes.")
	
	candidates = append(candidates, baseName)
	
	return candidates
}

// loadCUESchema attempts to load a CUE schema by name
func (d *CUESchemaDetector) loadCUESchema(schemaName string) (*schema.SchemaInfo, error) {
	if d.schemaManager == nil {
		return nil, fmt.Errorf("schema manager not available")
	}

	// Try to get schema from manager
	schemas, err := d.schemaManager.ListSchemas()
	if err != nil {
		return nil, fmt.Errorf("failed to list schemas: %w", err)
	}

	// Search through all schemas
	for packageName, packageSchemas := range schemas {
		for _, schemaInfo := range packageSchemas {
			// Check various name matches
			if d.matchesSchemaName(schemaName, packageName, schemaInfo.Name) {
				// Return the schema info directly
				return &schemaInfo, nil
			}
		}
	}

	return nil, fmt.Errorf("schema not found: %s", schemaName)
}

// matchesSchemaName checks if a schema name matches various patterns
func (d *CUESchemaDetector) matchesSchemaName(target, packageName, schemaName string) bool {
	// Direct match
	if target == schemaName {
		return true
	}
	
	// Package.schema match
	fullName := fmt.Sprintf("%s.%s", packageName, schemaName)
	if target == fullName {
		return true
	}
	
	// Case-insensitive match
	if strings.EqualFold(target, schemaName) {
		return true
	}
	
	// Hyphen/underscore variations
	normalizedTarget := strings.ReplaceAll(strings.ReplaceAll(target, "-", "_"), ".", "_")
	normalizedSchema := strings.ReplaceAll(strings.ReplaceAll(schemaName, "-", "_"), ".", "_")
	if normalizedTarget == normalizedSchema {
		return true
	}
	
	return false
}

// validateAgainstCUE validates detected objects against CUE schema
func (d *CUESchemaDetector) validateAgainstCUE(detection *SchemaDetection, cueSchema *schema.SchemaInfo) map[string]interface{} {
	// This is a placeholder for CUE validation
	// In a full implementation, this would:
	// 1. Convert detected objects to CUE values
	// 2. Validate against the CUE schema
	// 3. Return validation results

	result := map[string]interface{}{
		"schema_available": true,
		"schema_name":     cueSchema.Name,
		"schema_package":  cueSchema.Package,
	}

	// TODO: Implement actual CUE validation
	// This would require integrating with the CUE validation logic
	// from the existing schema package

	return result
}

// AddPatternFromCUE creates a detection pattern from a CUE schema
func (d *CUESchemaDetector) AddPatternFromCUE(cueSchema *schema.SchemaInfo) error {
	// This is a placeholder for converting CUE schemas to detection patterns
	// In a full implementation, this would:
	// 1. Parse the CUE schema definition
	// 2. Extract field requirements and types
	// 3. Create a SchemaPattern for the simple detector

	// For now, create a basic pattern based on schema metadata
	pattern := SchemaPattern{
		Name:        fmt.Sprintf("%s.%s", cueSchema.Package, cueSchema.Name),
		Description: fmt.Sprintf("CUE schema: %s", cueSchema.Name),
		Fields:      []FieldPattern{}, // TODO: Extract from CUE
		Optional:    []FieldPattern{}, // TODO: Extract from CUE
		Tags: map[string]string{
			"source":  "cue",
			"package": cueSchema.Package,
			"file":    filepath.Base(cueSchema.FilePath),
		},
	}

	d.simpleDetector.AddPattern(pattern)
	return nil
}

// LoadPatternsFromCUE loads detection patterns from all available CUE schemas
func (d *CUESchemaDetector) LoadPatternsFromCUE() error {
	if d.schemaManager == nil {
		return fmt.Errorf("schema manager not available")
	}
	
	schemas, err := d.schemaManager.ListSchemas()
	if err != nil {
		return fmt.Errorf("failed to list schemas: %w", err)
	}
	
	count := 0
	for _, packageSchemas := range schemas {
		for _, schemaInfo := range packageSchemas {
			// Add pattern from CUE schema info
			if err := d.AddPatternFromCUE(&schemaInfo); err != nil {
				continue // Skip patterns that can't be created
			}
			
			count++
		}
	}
	
	return nil
}

// GetCUESchemas returns information about available CUE schemas
func (d *CUESchemaDetector) GetCUESchemas() (map[string][]string, error) {
	if d.schemaManager == nil {
		return nil, fmt.Errorf("schema manager not available")
	}
	
	schemas, err := d.schemaManager.ListSchemas()
	if err != nil {
		return nil, fmt.Errorf("failed to list schemas: %w", err)
	}
	
	result := make(map[string][]string)
	for packageName, packageSchemas := range schemas {
		schemaNames := make([]string, len(packageSchemas))
		for i, schemaInfo := range packageSchemas {
			schemaNames[i] = schemaInfo.Name
		}
		result[packageName] = schemaNames
	}
	
	return result, nil
}
