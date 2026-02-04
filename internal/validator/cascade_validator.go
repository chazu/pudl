package validator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// CascadeValidator handles cascading schema validation with full CUE module support
type CascadeValidator struct {
	ctx        *cue.Context
	loader     *CUEModuleLoader
	modules    map[string]*LoadedModule
	schemas    map[string]cue.Value      // Flattened schema map for quick access
	metadata   map[string]SchemaMetadata // Flattened metadata map for quick access
	schemaPath string
}

// NewCascadeValidator creates a new cascade validator with full CUE module support
//
// This implementation uses CUE's official load package to properly handle:
// - Cross-references between schemas within the same package
// - Schema inheritance (e.g., CompliantEC2Instance inheriting from EC2Instance)
// - Proper CUE module compilation with all dependencies resolved
//
// The loading process:
// 1. Discovers all package directories in the schema path
// 2. Uses CUE's load.Instances to load each package as a unified module
// 3. Extracts all schema definitions and metadata from loaded modules
// 4. Validates module integrity including cross-reference consistency
func NewCascadeValidator(schemaPath string) (*CascadeValidator, error) {
	ctx := cuecontext.New()

	// Create the CUE module loader for proper cross-reference handling
	loader := NewCUEModuleLoader(schemaPath)

	// Load all CUE modules with proper cross-reference support
	// This replaces the previous individual file compilation approach
	modules, err := loader.LoadAllModules()
	if err != nil {
		return nil, fmt.Errorf("failed to load CUE modules: %w", err)
	}

	// Validate module integrity (check cross-references, base schema existence, etc.)
	if err := loader.ValidateModuleIntegrity(modules); err != nil {
		return nil, fmt.Errorf("module integrity validation failed: %w", err)
	}

	// Create flattened maps for quick access during validation
	// This maintains the same interface as before while supporting full module loading
	schemas := loader.GetAllSchemas(modules)
	metadata := loader.GetAllMetadata(modules)



	return &CascadeValidator{
		ctx:        ctx,
		loader:     loader,
		modules:    modules,
		schemas:    schemas,
		metadata:   metadata,
		schemaPath: schemaPath,
	}, nil
}

// ValidateWithCascade performs cascading validation against multiple schemas
func (cv *CascadeValidator) ValidateWithCascade(data interface{}, intendedSchema string) (*ValidationResult, error) {
	result := NewValidationResult(intendedSchema)
	
	// Get cascade chain for intended schema
	cascadeChain := cv.getCascadeChain(intendedSchema)
	
	// Convert data to CUE value using JSON encoding to handle type conversions properly
	var dataValue cue.Value

	// If data is already a map[string]interface{} from JSON parsing, re-encode it as JSON
	// and let CUE parse it to handle number types correctly
	if dataMap, ok := data.(map[string]interface{}); ok {
		jsonBytes, err := json.Marshal(dataMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal data to JSON: %w", err)
		}
		dataValue = cv.ctx.CompileBytes(jsonBytes)
	} else {
		// For other data types, use direct encoding
		dataValue = cv.ctx.Encode(data)
	}

	if dataValue.Err() != nil {
		return nil, fmt.Errorf("failed to encode data: %w", dataValue.Err())
	}
	
	// Try each schema in cascade order
	for i, schemaName := range cascadeChain {
		schema, exists := cv.schemas[schemaName]
		if !exists {
			result.AddCascadeAttempt(schemaName, false, nil, "Schema not found")
			continue
		}

		// Attempt validation by unifying data with schema
		unified := schema.Unify(dataValue)
		if err := unified.Validate(); err == nil {
			// Validation succeeded
			cascadeLevel := cv.determineCascadeLevel(i, len(cascadeChain))
			fallbackReason := cv.determineFallbackReason(i, intendedSchema, schemaName)

			result.SetFinalAssignment(schemaName, cascadeLevel, fallbackReason)
			result.AddCascadeAttempt(schemaName, true, nil, "Validation successful")

			return result, nil
		} else {
			// Validation failed - collect errors and continue cascade
			validationErrors := cv.extractValidationErrors(err, schemaName)
			reason := fmt.Sprintf("Validation failed: %d errors", len(validationErrors))
			result.AddCascadeAttempt(schemaName, false, validationErrors, reason)
		}
	}
	
	// Should never reach here due to catchall, but handle gracefully
	result.SetFinalAssignment("core.#CatchAll", "catchall", "All validations failed")
	
	return result, nil
}

// getCascadeChain builds the cascade chain for a given schema
func (cv *CascadeValidator) getCascadeChain(intendedSchema string) []string {
	meta, exists := cv.metadata[intendedSchema]
	if !exists {
		// Default cascade chain for unknown schemas
		return []string{intendedSchema, "core.#CatchAll"}
	}

	if len(meta.CascadeFallback) > 0 {
		// Use explicit cascade chain from schema metadata
		chain := []string{intendedSchema}
		chain = append(chain, meta.CascadeFallback...)
		return chain
	}

	// Build cascade chain based on schema metadata
	chain := []string{intendedSchema}

	// Add base schema if specified
	if meta.BaseSchema != "" {
		chain = append(chain, meta.BaseSchema)
	}

	// Add generic fallbacks based on resource type
	if meta.ResourceType != "" {
		parts := strings.Split(meta.ResourceType, ".")
		if len(parts) >= 2 {
			// Add generic resource schema (e.g., aws.#Resource)
			genericSchema := fmt.Sprintf("%s.#Resource", parts[0])
			if genericSchema != intendedSchema && !contains(chain, genericSchema) {
				chain = append(chain, genericSchema)
			}
		}
	}

	// Always end with catchall
	if !contains(chain, "core.#CatchAll") {
		chain = append(chain, "core.#CatchAll")
	}

	return chain
}

// determineCascadeLevel determines the cascade level based on position in chain
func (cv *CascadeValidator) determineCascadeLevel(position, chainLength int) string {
	if position == 0 {
		return "exact"
	}
	if position == chainLength-1 {
		return "catchall"
	}
	return "fallback"
}

// determineFallbackReason creates a human-readable fallback reason
func (cv *CascadeValidator) determineFallbackReason(position int, intendedSchema, assignedSchema string) string {
	if position == 0 {
		return "" // No fallback occurred
	}
	
	if assignedSchema == "core.#CatchAll" {
		return "Failed all specific schema validations"
	}
	
	return fmt.Sprintf("Failed validation against %s", intendedSchema)
}

// extractValidationErrors converts CUE validation errors to structured format
func (cv *CascadeValidator) extractValidationErrors(err error, schemaName string) []ValidationError {
	var errors []ValidationError

	// Use CUEErrorParser to parse errors into structured format
	parser := NewCUEErrorParser()
	parsedErrors := parser.Parse(err)

	// Convert parsed errors to ValidationError format
	for _, pe := range parsedErrors {
		errors = append(errors, ValidationError{
			Path:       pe.Path,
			Message:    pe.Constraint,
			SchemaName: schemaName,
			Constraint: pe.Constraint,
			Value:      pe.Got,
		})
	}

	// If no errors were parsed, create a generic error
	if len(errors) == 0 {
		errors = append(errors, ValidationError{
			Path:       "root",
			Message:    err.Error(),
			SchemaName: schemaName,
			Constraint: "unknown",
		})
	}

	return errors
}

// GetAvailableSchemas returns all loaded schemas
func (cv *CascadeValidator) GetAvailableSchemas() []string {
	var schemas []string
	for name := range cv.schemas {
		schemas = append(schemas, name)
	}
	sort.Strings(schemas)
	return schemas
}

// GetSchemaMetadata returns metadata for a specific schema
func (cv *CascadeValidator) GetSchemaMetadata(schemaName string) (SchemaMetadata, bool) {
	meta, exists := cv.metadata[schemaName]
	return meta, exists
}

// GetSchemasByType returns schemas filtered by type
func (cv *CascadeValidator) GetSchemasByType(schemaType string) []string {
	var schemas []string
	for name, meta := range cv.metadata {
		if meta.SchemaType == schemaType {
			schemas = append(schemas, name)
		}
	}
	sort.Strings(schemas)
	return schemas
}

// GetSchemasByResourceType returns schemas for a specific resource type
func (cv *CascadeValidator) GetSchemasByResourceType(resourceType string) []string {
	var schemas []string
	for name, meta := range cv.metadata {
		if meta.ResourceType == resourceType {
			schemas = append(schemas, name)
		}
	}
	
	// Sort by cascade priority (higher priority first)
	sort.Slice(schemas, func(i, j int) bool {
		metaI := cv.metadata[schemas[i]]
		metaJ := cv.metadata[schemas[j]]
		return metaI.CascadePriority > metaJ.CascadePriority
	})
	
	return schemas
}

// ResolveSchemaName attempts to resolve user-friendly schema names to full names
func (cv *CascadeValidator) ResolveSchemaName(userInput string) (string, error) {
	// If it's already a full schema name, return as-is
	if _, exists := cv.schemas[userInput]; exists {
		return userInput, nil
	}

	// Try to find matching schemas
	var matches []string
	for schemaName := range cv.schemas {
		// Check for exact match
		if schemaName == userInput {
			return schemaName, nil
		}

		// Check for partial matches
		parts := strings.Split(schemaName, ".")
		if len(parts) >= 2 {
			packageName := parts[0]
			definitionName := strings.TrimPrefix(parts[1], "#")

			// Try various matching patterns:
			// aws.compliant-ec2 -> aws.#CompliantEC2Instance
			// aws.ec2 -> aws.#EC2Instance
			// aws.#EC2Instance -> aws.#EC2Instance (exact)

			possibleInputs := []string{
				fmt.Sprintf("%s.%s", packageName, strings.ToLower(definitionName)),
				fmt.Sprintf("%s.%s", packageName, definitionName),
				fmt.Sprintf("%s.%s", packageName, strings.ToLower(strings.ReplaceAll(definitionName, "Instance", ""))),
				fmt.Sprintf("%s.%s", packageName, strings.ReplaceAll(definitionName, "Instance", "")),
			}

			// Also try kebab-case conversion for CompliantEC2Instance -> compliant-ec2
			kebabCase := definitionName
			kebabCase = strings.ReplaceAll(kebabCase, "CompliantEC2Instance", "compliant-ec2")
			kebabCase = strings.ReplaceAll(kebabCase, "EC2Instance", "ec2")
			kebabCase = strings.ReplaceAll(kebabCase, "RDSInstance", "rds-instance")
			kebabCase = strings.ToLower(kebabCase)
			if kebabCase != strings.ToLower(definitionName) {
				possibleInputs = append(possibleInputs, fmt.Sprintf("%s.%s", packageName, kebabCase))
			}

			for _, possible := range possibleInputs {
				if userInput == possible {
					matches = append(matches, schemaName)
					break
				}
			}
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}

	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous schema name '%s', matches: %s", userInput, strings.Join(matches, ", "))
	}

	// Show available schemas for debugging
	var availableSchemas []string
	for schemaName := range cv.schemas {
		availableSchemas = append(availableSchemas, schemaName)
	}

	return "", fmt.Errorf("schema not found: %s\nAvailable schemas: %s", userInput, strings.Join(availableSchemas, ", "))
}

// Helper functions

func extractPackageFromPath(schemaPath, filePath string) string {
	relPath, err := filepath.Rel(schemaPath, filePath)
	if err != nil {
		return "unknown"
	}
	
	dir := filepath.Dir(relPath)
	if dir == "." {
		return "root"
	}
	
	return strings.ReplaceAll(dir, string(filepath.Separator), ".")
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetLoadedModules returns the loaded CUE modules for inspection
func (cv *CascadeValidator) GetLoadedModules() map[string]*LoadedModule {
	return cv.modules
}

// GetModuleInfo returns information about a specific loaded module
func (cv *CascadeValidator) GetModuleInfo(packageName string) (*LoadedModule, error) {
	return cv.loader.GetModuleInfo(cv.modules, packageName)
}

// ReloadModules reloads all CUE modules (useful for development/testing)
func (cv *CascadeValidator) ReloadModules() error {
	modules, err := cv.loader.LoadAllModules()
	if err != nil {
		return fmt.Errorf("failed to reload CUE modules: %w", err)
	}

	if err := cv.loader.ValidateModuleIntegrity(modules); err != nil {
		return fmt.Errorf("module integrity validation failed after reload: %w", err)
	}

	// Update the validator with reloaded modules
	cv.modules = modules
	cv.schemas = cv.loader.GetAllSchemas(modules)
	cv.metadata = cv.loader.GetAllMetadata(modules)

	return nil
}
