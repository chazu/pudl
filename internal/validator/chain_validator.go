package validator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/schemaname"
)

// ChainValidator handles chained schema validation with full CUE module support
type ChainValidator struct {
	loaders     []*CUEModuleLoader
	modules     map[string]*LoadedModule
	schemas     map[string]cue.Value      // Flattened schema map for quick access
	metadata    map[string]SchemaMetadata // Flattened metadata map for quick access
	schemaPaths []string
	loadErrors  []SchemaLoadError
	cycles      [][]string
}

// NewChainValidator creates a new chain validator with full CUE module support.
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
//
// A package that fails to load, or a schema that fails integrity checks, is
// reported by LoadErrors rather than discarding its whole schema path.
func NewChainValidator(schemaPaths ...string) (*ChainValidator, error) {
	if len(schemaPaths) == 0 {
		return nil, fmt.Errorf("at least one schema path is required")
	}

	cv := &ChainValidator{schemaPaths: schemaPaths}
	cv.apply(LoadSchemaSet(schemaPaths))
	return cv, nil
}

// findFallbackSchemaName finds the actual schema name for the Item/catchall schema
// from the loaded schemas map. Returns canonical format (e.g., "pudl/core.#Item").
func (cv *ChainValidator) findFallbackSchemaName() string {
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

// ValidateChain validates data against the intended schema using CUE unification.
// If the intended schema fails, it tries the base schema (if any), then the catchall.
func (cv *ChainValidator) ValidateChain(data interface{}, intendedSchema string) (*ValidationResult, error) {
	// Normalize the intended schema to canonical format
	intendedSchema = schemaname.Normalize(intendedSchema)
	result := NewValidationResult(intendedSchema)

	// Build validation chain: intended → base (if any) → catchall
	chain, err := cv.buildValidationChain(intendedSchema)
	if err != nil {
		return nil, err
	}

	// Convert data to CUE value using JSON encoding for proper type handling
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data to JSON: %w", err)
	}

	// Schemas from different search paths live in different CUE contexts, and
	// CUE only allows values from the same context in one operation. Compile the
	// data once per schema context.
	dataByCtx := make(map[*cue.Context]cue.Value)

	// Try each schema in chain via CUE unification
	for _, schemaName := range chain {
		schema, exists := cv.schemas[schemaName]
		if !exists {
			result.AddChainAttempt(schemaName, false, nil, "Schema not found")
			continue
		}

		schemaCtx := schema.Context()
		dataValue, compiled := dataByCtx[schemaCtx]
		if !compiled {
			dataValue = schemaCtx.CompileBytes(jsonBytes)
			if dataValue.Err() != nil {
				return nil, fmt.Errorf("failed to encode data: %w", dataValue.Err())
			}
			dataByCtx[schemaCtx] = dataValue
		}

		unified := schema.Unify(dataValue)
		if err := unified.Validate(); err == nil {
			fallbackReason := ""
			if schemaName != intendedSchema {
				fallbackReason = fmt.Sprintf("Failed validation against %s", intendedSchema)
			}
			result.SetFinalAssignment(schemaName, fallbackReason)
			result.AddChainAttempt(schemaName, true, nil, "Validation successful")
			return result, nil
		} else {
			validationErrors := cv.extractValidationErrors(err, schemaName)
			reason := fmt.Sprintf("Validation failed: %d errors", len(validationErrors))
			result.AddChainAttempt(schemaName, false, validationErrors, reason)
		}
	}

	// Should never reach here due to catchall, but handle gracefully
	fallbackSchema := cv.findFallbackSchemaName()
	result.SetFinalAssignment(fallbackSchema, "All validations failed")

	return result, nil
}

// buildValidationChain builds the validation chain: intended → base (if any) → catchall.
// Uses CUE's natural inheritance via base_schema references.
//
// A base_schema cycle is an error: there is no family root to fall back to, and
// walking the chain would never end.
func (cv *ChainValidator) buildValidationChain(intendedSchema string) ([]string, error) {
	fallbackSchema := cv.findFallbackSchemaName()

	chain := []string{intendedSchema}
	seen := map[string]bool{intendedSchema: true}

	// Walk up the base schema chain
	current := intendedSchema
	for {
		meta, exists := cv.metadata[current]
		if !exists || meta.BaseSchema == "" {
			break
		}
		if seen[meta.BaseSchema] {
			return nil, fmt.Errorf("base_schema cycle: %s", FormatCycle(append(chain, meta.BaseSchema)))
		}
		seen[meta.BaseSchema] = true
		chain = append(chain, meta.BaseSchema)
		current = meta.BaseSchema
	}

	// Always end with catchall
	if !contains(chain, fallbackSchema) {
		chain = append(chain, fallbackSchema)
	}

	return chain, nil
}

// extractValidationErrors converts CUE validation errors to structured format
func (cv *ChainValidator) extractValidationErrors(err error, schemaName string) []ValidationError {
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
func (cv *ChainValidator) GetAvailableSchemas() []string {
	var schemas []string
	for name := range cv.schemas {
		schemas = append(schemas, name)
	}
	sort.Strings(schemas)
	return schemas
}

// HasSchema reports whether a canonical or legacy schema reference is loaded.
// Callers that persist typed assignments should use this before accepting a
// reference so a missing schema cannot silently become a generic fallback.
func (cv *ChainValidator) HasSchema(schemaName string) bool {
	_, exists := cv.schemas[schemaname.Normalize(schemaName)]
	return exists
}

// GetSchemaMetadata returns metadata for a specific schema
func (cv *ChainValidator) GetSchemaMetadata(schemaName string) (SchemaMetadata, bool) {
	meta, exists := cv.metadata[schemaName]
	return meta, exists
}

// GetSchemasByType returns schemas filtered by type
func (cv *ChainValidator) GetSchemasByType(schemaType string) []string {
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
func (cv *ChainValidator) GetSchemasByResourceType(resourceType string) []string {
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
func (cv *ChainValidator) ResolveSchemaName(userInput string) (string, error) {
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
func (cv *ChainValidator) GetLoadedModules() map[string]*LoadedModule {
	return cv.modules
}

// GetModuleInfo returns information about a specific loaded module
func (cv *ChainValidator) GetModuleInfo(packageName string) (*LoadedModule, error) {
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
func (cv *ChainValidator) ReloadModules() error {
	cv.apply(LoadSchemaSet(cv.schemaPaths))
	return nil
}

// apply installs a freshly loaded schema set.
func (cv *ChainValidator) apply(set *SchemaSet) {
	cv.loaders = set.Loaders
	cv.modules = set.Modules
	cv.schemas = set.Schemas
	cv.metadata = set.Metadata
	cv.loadErrors = set.Errors
	cv.cycles = set.Cycles
}

// LoadErrors reports the schema packages that failed to load, and loaded
// schemas that failed integrity checks, as of the last load.
func (cv *ChainValidator) LoadErrors() []SchemaLoadError {
	return append([]SchemaLoadError(nil), cv.loadErrors...)
}

// BaseSchemaCycles reports base_schema cycles among the loaded schemas.
func (cv *ChainValidator) BaseSchemaCycles() [][]string {
	return append([][]string(nil), cv.cycles...)
}
