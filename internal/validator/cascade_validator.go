package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// CascadeValidator handles cascading schema validation
type CascadeValidator struct {
	ctx      *cue.Context
	schemas  map[string]cue.Value
	metadata map[string]SchemaMetadata
}

// NewCascadeValidator creates a new cascade validator
func NewCascadeValidator(schemaPath string) (*CascadeValidator, error) {
	ctx := cuecontext.New()

	schemas := make(map[string]cue.Value)
	metadata := make(map[string]SchemaMetadata)

	// For now, let's use a simpler approach and load individual files
	// We'll skip the compliant schema that has cross-references for now
	err := filepath.Walk(schemaPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(info.Name(), ".cue") || info.IsDir() {
			return nil
		}

		// Skip compliant schemas for now (they have cross-references)
		// TODO: Fix cross-reference handling
		if strings.Contains(info.Name(), "compliant") {
			return nil
		}

		// Load and compile the CUE file
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		value := ctx.CompileString(string(content), cue.Filename(path))
		if value.Err() != nil {
			return fmt.Errorf("failed to compile %s: %w", path, value.Err())
		}

		// Extract package name from the file
		packageName := extractPackageFromPath(schemaPath, path)

		// Extract schema definitions and metadata
		iter, _ := value.Fields(cue.Definitions(true))
		for iter.Next() {
			label := iter.Label()
			if !strings.HasPrefix(label, "#") {
				continue
			}

			schemaValue := iter.Value()
			schemaName := fmt.Sprintf("%s.%s", packageName, label)
			schemas[schemaName] = schemaValue

			// Extract PUDL metadata if present
			if pudlMeta := schemaValue.LookupPath(cue.ParsePath("_pudl")); pudlMeta.Exists() {
				var meta SchemaMetadata
				if err := pudlMeta.Decode(&meta); err == nil {
					metadata[schemaName] = meta
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load schemas: %w", err)
	}



	return &CascadeValidator{
		ctx:      ctx,
		schemas:  schemas,
		metadata: metadata,
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
	result.SetFinalAssignment("unknown.#CatchAll", "catchall", "All validations failed")
	
	return result, nil
}

// getCascadeChain builds the cascade chain for a given schema
func (cv *CascadeValidator) getCascadeChain(intendedSchema string) []string {
	meta, exists := cv.metadata[intendedSchema]
	if !exists {
		// Default cascade chain for unknown schemas
		return []string{intendedSchema, "unknown.#CatchAll"}
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
	if !contains(chain, "unknown.#CatchAll") {
		chain = append(chain, "unknown.#CatchAll")
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
	
	if assignedSchema == "unknown.#CatchAll" {
		return "Failed all specific schema validations"
	}
	
	return fmt.Sprintf("Failed validation against %s", intendedSchema)
}

// extractValidationErrors converts CUE validation errors to structured format
func (cv *CascadeValidator) extractValidationErrors(err error, schemaName string) []ValidationError {
	var errors []ValidationError
	
	// Parse CUE error message
	errorMsg := err.Error()
	
	// For now, create a simple error structure
	// TODO: Enhance this to parse CUE errors more precisely
	errors = append(errors, ValidationError{
		Path:       "root",
		Message:    errorMsg,
		SchemaName: schemaName,
		Constraint: "unknown",
	})
	
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
