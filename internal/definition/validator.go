package definition

import (
	"fmt"

	"pudl/internal/validator"
)

// Validator validates definitions using the CUE module loader.
type Validator struct {
	schemaPath string
	discoverer *Discoverer
}

// NewValidator creates a new definition validator.
func NewValidator(schemaPath string) *Validator {
	return &Validator{
		schemaPath: schemaPath,
		discoverer: NewDiscoverer(schemaPath),
	}
}

// ValidateDefinition validates a specific definition by name.
func (v *Validator) ValidateDefinition(name string) (*ValidationResult, error) {
	def, err := v.discoverer.GetDefinition(name)
	if err != nil {
		return nil, err
	}

	result := &ValidationResult{
		Name:  def.Name,
		Valid: true,
	}

	// Load CUE modules and check for errors
	loader := validator.NewCUEModuleLoader(v.schemaPath)
	_, err = loader.LoadAllModules()
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("CUE load error: %v", err))
		return result, nil
	}

	// If modules loaded without error, the definition is valid
	// (CUE evaluates all files including definitions during load)
	def.Validated = true
	result.Valid = true

	return result, nil
}

// ValidateAll validates all definitions and returns results.
func (v *Validator) ValidateAll() ([]ValidationResult, error) {
	definitions, err := v.discoverer.ListDefinitions()
	if err != nil {
		return nil, err
	}

	if len(definitions) == 0 {
		return nil, nil
	}

	// Load CUE modules once for all definitions
	loader := validator.NewCUEModuleLoader(v.schemaPath)
	_, loadErr := loader.LoadAllModules()

	var results []ValidationResult
	for _, def := range definitions {
		result := ValidationResult{
			Name:  def.Name,
			Valid: true,
		}

		if loadErr != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("CUE load error: %v", loadErr))
		}

		results = append(results, result)
	}

	return results, nil
}
