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
	// Create cascade validator with full CUE module support
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
	// Use the cascade validator to validate data against the intended schema
	result, err := vs.cascadeValidator.ValidateWithCascade(data, schemaName)
	if err != nil {
		return &ServiceValidationResult{
			Valid:        false,
			SchemaName:   schemaName,
			ErrorMessage: fmt.Sprintf("Validation system error: %v", err),
			Errors:       []string{err.Error()},
		}
	}

	// Convert cascade validation result to service validation result
	return vs.convertCascadeResult(result, schemaName)
}

// convertCascadeResult converts a cascade validation result to a service validation result
func (vs *ValidationService) convertCascadeResult(cascadeResult *ValidationResult, intendedSchema string) *ServiceValidationResult {
	result := &ServiceValidationResult{
		SchemaName:       intendedSchema,
		AssignedSchema:   cascadeResult.AssignedSchema,
		CascadeLevel:     cascadeResult.CascadeLevel,
		ComplianceStatus: cascadeResult.GetComplianceStatus(),
		FallbackReason:   cascadeResult.FallbackReason,
	}

	// Determine if validation was successful
	if cascadeResult.AssignedSchema == intendedSchema {
		// Data validates against the intended schema
		result.Valid = true
		result.ErrorMessage = ""
	} else {
		// Data was assigned to a different schema (fallback occurred)
		result.Valid = false
		result.ErrorMessage = vs.buildFallbackMessage(cascadeResult)
	}

	// Convert validation errors
	result.Errors = vs.convertValidationErrors(cascadeResult.ValidationErrors)

	// Add cascade attempt details
	result.CascadeAttempts = vs.convertCascadeAttempts(cascadeResult.CascadeAttempts)

	return result
}

// buildFallbackMessage creates a user-friendly message explaining why validation failed
func (vs *ValidationService) buildFallbackMessage(cascadeResult *ValidationResult) string {
	if cascadeResult.FallbackReason != "" {
		return cascadeResult.FallbackReason
	}

	switch cascadeResult.CascadeLevel {
	case "fallback":
		return fmt.Sprintf("Data doesn't match %s but validates against %s",
			cascadeResult.IntendedSchema, cascadeResult.AssignedSchema)
	case "catchall":
		return fmt.Sprintf("Data doesn't match %s and was assigned to catchall schema %s",
			cascadeResult.IntendedSchema, cascadeResult.AssignedSchema)
	default:
		return fmt.Sprintf("Data was assigned to %s instead of %s",
			cascadeResult.AssignedSchema, cascadeResult.IntendedSchema)
	}
}

// convertValidationErrors converts cascade validation errors to string errors
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
		return fmt.Sprintf("✅ Data validates against schema %s", result.SchemaName)
	}

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("❌ Validation failed for schema %s\n", result.SchemaName))

	if result.AssignedSchema != "" && result.AssignedSchema != result.SchemaName {
		summary.WriteString(fmt.Sprintf("   → Data was assigned to %s instead\n", result.AssignedSchema))
	}

	if result.ErrorMessage != "" {
		summary.WriteString(fmt.Sprintf("   → %s\n", result.ErrorMessage))
	}

	if len(result.Errors) > 0 {
		summary.WriteString(fmt.Sprintf("   → %d validation errors found\n", len(result.Errors)))

		// Show first few errors
		maxErrors := 3
		for i, err := range result.Errors {
			if i >= maxErrors {
				summary.WriteString(fmt.Sprintf("   → ... and %d more errors\n", len(result.Errors)-maxErrors))
				break
			}
			summary.WriteString(fmt.Sprintf("   → %s\n", err))
		}
	}

	return summary.String()
}

// GetDetailedValidationReport returns a detailed validation report
func (vs *ValidationService) GetDetailedValidationReport(result *ServiceValidationResult) string {
	var report strings.Builder

	report.WriteString("📋 Detailed Validation Report\n")
	report.WriteString("═══════════════════════════════════════════════════════════════\n")
	report.WriteString(fmt.Sprintf("Intended Schema: %s\n", result.SchemaName))
	report.WriteString(fmt.Sprintf("Assigned Schema: %s\n", result.AssignedSchema))
	report.WriteString(fmt.Sprintf("Cascade Level:   %s\n", result.CascadeLevel))
	report.WriteString(fmt.Sprintf("Compliance:      %s\n", result.ComplianceStatus))
	report.WriteString(fmt.Sprintf("Valid:           %t\n", result.Valid))

	if result.FallbackReason != "" {
		report.WriteString(fmt.Sprintf("Fallback Reason: %s\n", result.FallbackReason))
	}

	report.WriteString("\n")

	if len(result.CascadeAttempts) > 0 {
		report.WriteString("🔄 Cascade Attempts:\n")
		report.WriteString("───────────────────────────────────────────────────────────────\n")
		for i, attempt := range result.CascadeAttempts {
			status := "❌"
			if attempt.Success {
				status = "✅"
			}
			report.WriteString(fmt.Sprintf("%d. %s %s - %s\n", i+1, status, attempt.SchemaName, attempt.Reason))

			if len(attempt.Errors) > 0 {
				for _, err := range attempt.Errors {
					report.WriteString(fmt.Sprintf("   • %s\n", err))
				}
			}
		}
		report.WriteString("\n")
	}

	if len(result.Errors) > 0 {
		report.WriteString("❌ Validation Errors:\n")
		report.WriteString("───────────────────────────────────────────────────────────────\n")
		for i, err := range result.Errors {
			report.WriteString(fmt.Sprintf("%d. %s\n", i+1, err))
		}
	}

	return report.String()
}

// ServiceValidationResult represents the result of validating data against a schema
type ServiceValidationResult struct {
	Valid            bool                   `json:"valid"`
	SchemaName       string                 `json:"schema_name"`
	AssignedSchema   string                 `json:"assigned_schema"`
	CascadeLevel     string                 `json:"cascade_level"`
	ComplianceStatus string                 `json:"compliance_status"`
	ErrorMessage     string                 `json:"error_message"`
	Errors           []string               `json:"errors"`
	FallbackReason   string                 `json:"fallback_reason"`
	CascadeAttempts  []ServiceCascadeAttempt `json:"cascade_attempts"`
}

// ServiceCascadeAttempt represents a single validation attempt in the cascade
type ServiceCascadeAttempt struct {
	SchemaName string   `json:"schema_name"`
	Success    bool     `json:"success"`
	Reason     string   `json:"reason"`
	Errors     []string `json:"errors"`
}

// IsCompliant returns true if the data is compliant with the intended schema
func (vr *ServiceValidationResult) IsCompliant() bool {
	return vr.ComplianceStatus == "compliant"
}

// IsNonCompliant returns true if the data is valid but doesn't meet business rules
func (vr *ServiceValidationResult) IsNonCompliant() bool {
	return vr.ComplianceStatus == "non-compliant"
}

// IsUnknown returns true if the data couldn't be classified properly
func (vr *ServiceValidationResult) IsUnknown() bool {
	return vr.ComplianceStatus == "unknown"
}

// HasFallback returns true if the data was assigned to a different schema than intended
func (vr *ServiceValidationResult) HasFallback() bool {
	return vr.AssignedSchema != vr.SchemaName
}

// GetSeverity returns the severity level of the validation result
func (vr *ServiceValidationResult) GetSeverity() string {
	if vr.Valid && vr.IsCompliant() {
		return "success"
	}
	if vr.Valid && vr.IsNonCompliant() {
		return "warning"
	}
	if vr.Valid && vr.IsUnknown() {
		return "info"
	}
	return "error"
}
