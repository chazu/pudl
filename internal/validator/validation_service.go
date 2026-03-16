package validator

import (
	"fmt"
	"strings"

	"pudl/internal/errors"
)

// ValidationService handles CUE validation for data validation workflows
type ValidationService struct {
	cascadeValidator *CascadeValidator
	schemaPath       string
}

// NewValidationService creates a new validation service
func NewValidationService(schemaPath string) (*ValidationService, error) {
	cascadeValidator, err := NewCascadeValidator(schemaPath)
	if err != nil {
		return nil, errors.WrapError(
			errors.ErrCodeValidationFailed,
			"Failed to initialize CUE validation system",
			err,
		)
	}

	return &ValidationService{
		cascadeValidator: cascadeValidator,
		schemaPath:       schemaPath,
	}, nil
}

// ValidateDataAgainstSchema validates data against a specific schema using CUE
func (vs *ValidationService) ValidateDataAgainstSchema(data interface{}, schemaName string) *ServiceValidationResult {
	result, err := vs.cascadeValidator.ValidateWithCascade(data, schemaName)
	if err != nil {
		return &ServiceValidationResult{
			Valid:        false,
			SchemaName:   schemaName,
			ErrorMessage: fmt.Sprintf("Validation system error: %v", err),
			Errors:       []string{err.Error()},
		}
	}

	return vs.convertResult(result, schemaName)
}

// convertResult converts a validation result to a service validation result
func (vs *ValidationService) convertResult(vr *ValidationResult, intendedSchema string) *ServiceValidationResult {
	result := &ServiceValidationResult{
		SchemaName:     intendedSchema,
		AssignedSchema: vr.AssignedSchema,
		Valid:          vr.Valid,
		FallbackReason: vr.FallbackReason,
	}

	if !vr.Valid {
		result.ErrorMessage = vs.buildFallbackMessage(vr)
	}

	result.Errors = vs.convertValidationErrors(vr.ValidationErrors)
	result.CascadeAttempts = vs.convertCascadeAttempts(vr.CascadeAttempts)

	return result
}

// buildFallbackMessage creates a user-friendly message explaining why validation failed
func (vs *ValidationService) buildFallbackMessage(vr *ValidationResult) string {
	if vr.FallbackReason != "" {
		return vr.FallbackReason
	}
	if vr.AssignedSchema != vr.IntendedSchema {
		return fmt.Sprintf("Data doesn't match %s, assigned to %s",
			vr.IntendedSchema, vr.AssignedSchema)
	}
	return ""
}

// convertValidationErrors converts validation errors to string errors
func (vs *ValidationService) convertValidationErrors(cascadeErrors []ValidationError) []string {
	var errs []string
	for _, err := range cascadeErrors {
		errorMsg := fmt.Sprintf("Path '%s': %s", err.Path, err.Message)
		if err.Constraint != "" {
			errorMsg += fmt.Sprintf(" (constraint: %s)", err.Constraint)
		}
		errs = append(errs, errorMsg)
	}
	return errs
}

// convertCascadeAttempts converts cascade attempts to service format
func (vs *ValidationService) convertCascadeAttempts(cascadeAttempts []CascadeAttempt) []ServiceCascadeAttempt {
	var attempts []ServiceCascadeAttempt
	for _, attempt := range cascadeAttempts {
		serviceAttempt := ServiceCascadeAttempt{
			SchemaName: attempt.SchemaName,
			Success:    attempt.Success,
			Reason:     attempt.Reason,
			Errors:     vs.convertValidationErrors(attempt.Errors),
		}
		attempts = append(attempts, serviceAttempt)
	}
	return attempts
}

// GetValidationSummary returns a human-readable summary of validation results
func (vs *ValidationService) GetValidationSummary(result *ServiceValidationResult) string {
	if result.Valid {
		return fmt.Sprintf("Data validates against schema %s", result.SchemaName)
	}

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Validation failed for schema %s\n", result.SchemaName))

	if result.AssignedSchema != "" && result.AssignedSchema != result.SchemaName {
		summary.WriteString(fmt.Sprintf("   -> Data was assigned to %s instead\n", result.AssignedSchema))
	}

	if result.ErrorMessage != "" {
		summary.WriteString(fmt.Sprintf("   -> %s\n", result.ErrorMessage))
	}

	if len(result.Errors) > 0 {
		summary.WriteString(fmt.Sprintf("   -> %d validation errors found\n", len(result.Errors)))
		maxErrors := 3
		for i, err := range result.Errors {
			if i >= maxErrors {
				summary.WriteString(fmt.Sprintf("   -> ... and %d more errors\n", len(result.Errors)-maxErrors))
				break
			}
			summary.WriteString(fmt.Sprintf("   -> %s\n", err))
		}
	}

	return summary.String()
}

// ServiceValidationResult represents the result of validating data against a schema
type ServiceValidationResult struct {
	Valid           bool                    `json:"valid"`
	SchemaName      string                  `json:"schema_name"`
	AssignedSchema  string                  `json:"assigned_schema"`
	ErrorMessage    string                  `json:"error_message"`
	Errors          []string                `json:"errors"`
	FallbackReason  string                  `json:"fallback_reason"`
	CascadeAttempts []ServiceCascadeAttempt  `json:"cascade_attempts"`
}

// ServiceCascadeAttempt represents a single validation attempt
type ServiceCascadeAttempt struct {
	SchemaName string   `json:"schema_name"`
	Success    bool     `json:"success"`
	Reason     string   `json:"reason"`
	Errors     []string `json:"errors"`
}

// HasFallback returns true if the data was assigned to a different schema than intended
func (vr *ServiceValidationResult) HasFallback() bool {
	return vr.AssignedSchema != vr.SchemaName
}
