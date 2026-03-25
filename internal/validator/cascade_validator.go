package validator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"pudl/internal/schemaname"
)

// CascadeValidator handles cascading schema validation with full CUE module support
type CascadeValidator struct {
	ctx         *cue.Context
	loaders     []*CUEModuleLoader
	modules     map[string]*LoadedModule
	schemas     map[string]cue.Value      // Flattened schema map for quick access
	metadata    map[string]SchemaMetadata // Flattened metadata map for quick access
	schemaPaths []string
}

// NewCascadeValidator creates a new cascade validator with full CUE module support.
// When multiple paths are provided, schemas are loaded in order; the first occurrence
// of a schema name wins (per-repo shadows global).
//
// This implementation uses CUE's official load package to properly handle:
// - Cross-references between schemas within the same package
// - Schema inheritance (e.g., CompliantEC2Instance inheriting from EC2Instance)
// - Proper CUE module compilation with all dependencies resolved
//
// The loading process:
// 1. Discovers all package directories in each schema path
// 2. Uses CUE's load.Instances to load each package as a unified module
// 3. Extracts all schema definitions and metadata from loaded modules
// 4. Validates module integrity including cross-reference consistency
func NewCascadeValidator(schemaPaths ...string) (*CascadeValidator, error) {
	if len(schemaPaths) == 0 {
		return nil, fmt.Errorf("at least one schema path is required")
	}

	ctx := cuecontext.New()

	allSchemas := make(map[string]cue.Value)
	allMetadata := make(map[string]SchemaMetadata)
	allModules := make(map[string]*LoadedModule)
	var allLoaders []*CUEModuleLoader

	for _, sp := range schemaPaths {
		loader := NewCUEModuleLoader(sp)
		allLoaders = append(allLoaders, loader)

		modules, err := loader.LoadAllModules()
		if err != nil {
			// Skip inaccessible or invalid schema directories
			continue
		}

		// Validate module integrity for this path
		if err := loader.ValidateModuleIntegrity(modules); err != nil {
			// Skip paths with integrity issues
			continue
		}

		schemas := loader.GetAllSchemas(modules)
		metadata := loader.GetAllMetadata(modules)

		// First-found wins: only add schemas not already seen
		for name, val := range schemas {
			if _, exists := allSchemas[name]; !exists {
				allSchemas[name] = val
			}
		}
		for name, meta := range metadata {
			if _, exists := allMetadata[name]; !exists {
				allMetadata[name] = meta
			}
		}
		for name, mod := range modules {
			if _, exists := allModules[name]; !exists {
				allModules[name] = mod
			}
		}
	}

	cv := &CascadeValidator{
		ctx:         ctx,
		loaders:     allLoaders,
		modules:     allModules,
		schemas:     allSchemas,
		metadata:    allMetadata,
		schemaPaths: schemaPaths,
	}

	return cv, nil
}

// findFallbackSchemaName finds the actual schema name for the Item/catchall schema
// from the loaded schemas map. Returns canonical format (e.g., "pudl/core.#Item").
func (cv *CascadeValidator) findFallbackSchemaName() string {
	// Canonical fallback schema name
	const fallbackCanonical = "pudl/core.#Item"

	// Check if the canonical name exists
	if _, exists := cv.schemas[fallbackCanonical]; exists {
		return fallbackCanonical
	}

	// Search for any schema that normalizes to the fallback
	for name := range cv.schemas {
		if schemaname.IsFallbackSchema(name) && strings.HasSuffix(name, "#Item") {
			return name
		}
	}

	// Return canonical format (will be caught as "Schema not found" if missing)
	return fallbackCanonical
}

// ValidateWithCascade validates data against the intended schema using CUE unification.
// If the intended schema fails, it tries the base schema (if any), then the catchall.
func (cv *CascadeValidator) ValidateWithCascade(data interface{}, intendedSchema string) (*ValidationResult, error) {
	// Normalize the intended schema to canonical format
	intendedSchema = schemaname.Normalize(intendedSchema)
	result := NewValidationResult(intendedSchema)

	// Build validation chain: intended → base (if any) → catchall
	chain := cv.buildValidationChain(intendedSchema)

	// Convert data to CUE value using JSON encoding for proper type handling
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data to JSON: %w", err)
	}
	dataValue := cv.ctx.CompileBytes(jsonBytes)

	if dataValue.Err() != nil {
		return nil, fmt.Errorf("failed to encode data: %w", dataValue.Err())
	}

	// Try each schema in chain via CUE unification
	for _, schemaName := range chain {
		schema, exists := cv.schemas[schemaName]
		if !exists {
			result.AddCascadeAttempt(schemaName, false, nil, "Schema not found")
			continue
		}

		unified := schema.Unify(dataValue)
		if err := unified.Validate(); err == nil {
			fallbackReason := ""
			if schemaName != intendedSchema {
				fallbackReason = fmt.Sprintf("Failed validation against %s", intendedSchema)
			}
			result.SetFinalAssignment(schemaName, fallbackReason)
			result.AddCascadeAttempt(schemaName, true, nil, "Validation successful")
			return result, nil
		} else {
			validationErrors := cv.extractValidationErrors(err, schemaName)
			reason := fmt.Sprintf("Validation failed: %d errors", len(validationErrors))
			result.AddCascadeAttempt(schemaName, false, validationErrors, reason)
		}
	}

	// Should never reach here due to catchall, but handle gracefully
	fallbackSchema := cv.findFallbackSchemaName()
	result.SetFinalAssignment(fallbackSchema, "All validations failed")

	return result, nil
}

// buildValidationChain builds the validation chain: intended → base (if any) → catchall.
// Uses CUE's natural inheritance via base_schema references.
func (cv *CascadeValidator) buildValidationChain(intendedSchema string) []string {
	fallbackSchema := cv.findFallbackSchemaName()

	chain := []string{intendedSchema}

	// Walk up the base schema chain
	current := intendedSchema
	for {
		meta, exists := cv.metadata[current]
		if !exists || meta.BaseSchema == "" {
			break
		}
		if !contains(chain, meta.BaseSchema) {
			chain = append(chain, meta.BaseSchema)
		}
		current = meta.BaseSchema
	}

	// Always end with catchall
	if !contains(chain, fallbackSchema) {
		chain = append(chain, fallbackSchema)
	}

	return chain
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
	sort.Strings(schemas)
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
	// Search through all loaders
	for _, loader := range cv.loaders {
		mod, err := loader.GetModuleInfo(cv.modules, packageName)
		if err == nil {
			return mod, nil
		}
	}
	return nil, fmt.Errorf("module %s not found", packageName)
}

// ReloadModules reloads all CUE modules (useful for development/testing)
func (cv *CascadeValidator) ReloadModules() error {
	allSchemas := make(map[string]cue.Value)
	allMetadata := make(map[string]SchemaMetadata)
	allModules := make(map[string]*LoadedModule)
	var allLoaders []*CUEModuleLoader

	for _, sp := range cv.schemaPaths {
		loader := NewCUEModuleLoader(sp)
		allLoaders = append(allLoaders, loader)

		modules, err := loader.LoadAllModules()
		if err != nil {
			continue
		}

		if err := loader.ValidateModuleIntegrity(modules); err != nil {
			continue
		}

		schemas := loader.GetAllSchemas(modules)
		metadata := loader.GetAllMetadata(modules)

		for name, val := range schemas {
			if _, exists := allSchemas[name]; !exists {
				allSchemas[name] = val
			}
		}
		for name, meta := range metadata {
			if _, exists := allMetadata[name]; !exists {
				allMetadata[name] = meta
			}
		}
		for name, mod := range modules {
			if _, exists := allModules[name]; !exists {
				allModules[name] = mod
			}
		}
	}

	cv.loaders = allLoaders
	cv.modules = allModules
	cv.schemas = allSchemas
	cv.metadata = allMetadata

	return nil
}
