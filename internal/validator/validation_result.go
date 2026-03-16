package validator

import (
	"fmt"
	"strings"
)

// ValidationResult represents the result of schema validation
type ValidationResult struct {
	IntendedSchema   string            `json:"intended_schema"`
	AssignedSchema   string            `json:"assigned_schema"`
	Valid            bool              `json:"valid"`             // Whether data matched the intended schema
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
	FallbackReason   string            `json:"fallback_reason,omitempty"`
	Success          bool              `json:"success"`           // Always true (never reject data)
	CascadeAttempts  []CascadeAttempt  `json:"cascade_attempts"`  // All validation attempts
}

// ValidationError represents a specific validation failure
type ValidationError struct {
	Path       string      `json:"path"`
	Message    string      `json:"message"`
	Value      interface{} `json:"value"`
	SchemaName string      `json:"schema_name"`
	Constraint string      `json:"constraint"`
}

// CascadeAttempt represents a single validation attempt
type CascadeAttempt struct {
	SchemaName string            `json:"schema_name"`
	Success    bool              `json:"success"`
	Errors     []ValidationError `json:"errors,omitempty"`
	Reason     string            `json:"reason,omitempty"`
}

// SchemaMetadata represents PUDL metadata embedded in CUE schemas
type SchemaMetadata struct {
	SchemaType     string   `json:"schema_type"`      // "base", "policy", "custom", "catchall"
	ResourceType   string   `json:"resource_type"`    // "aws.ec2.instance", "k8s.pod"
	BaseSchema     string   `json:"base_schema"`      // Parent schema reference
	IdentityFields []string `json:"identity_fields"`  // Fields that identify the resource
	TrackedFields  []string `json:"tracked_fields"`   // Fields to monitor for changes
	IsListType     bool     `json:"is_list_type"`     // True if schema is structurally a list/array type
}

// GetSummary returns a human-readable summary of the validation result
func (vr *ValidationResult) GetSummary() string {
	if vr.Valid {
		return fmt.Sprintf("Validated against %s", vr.AssignedSchema)
	}
	if vr.AssignedSchema != "" && vr.AssignedSchema != vr.IntendedSchema {
		return fmt.Sprintf("Does not match %s, assigned to %s", vr.IntendedSchema, vr.AssignedSchema)
	}
	return fmt.Sprintf("Assigned to catchall (intended: %s)", vr.IntendedSchema)
}

// GetDetailedReport returns a detailed validation report
func (vr *ValidationResult) GetDetailedReport() string {
	var report strings.Builder

	report.WriteString(fmt.Sprintf("Intended Schema: %s\n", vr.IntendedSchema))
	report.WriteString(fmt.Sprintf("Assigned Schema: %s\n", vr.AssignedSchema))
	report.WriteString(fmt.Sprintf("Valid: %t\n", vr.Valid))

	if vr.FallbackReason != "" {
		report.WriteString(fmt.Sprintf("Fallback Reason: %s\n", vr.FallbackReason))
	}

	if len(vr.CascadeAttempts) > 1 {
		report.WriteString("\nValidation Attempts:\n")
		for i, attempt := range vr.CascadeAttempts {
			status := "FAIL"
			if attempt.Success {
				status = "OK"
			}
			report.WriteString(fmt.Sprintf("   %d. [%s] %s", i+1, status, attempt.SchemaName))
			if attempt.Reason != "" {
				report.WriteString(fmt.Sprintf(" (%s)", attempt.Reason))
			}
			report.WriteString("\n")
		}
	}

	if len(vr.ValidationErrors) > 0 {
		report.WriteString("\nValidation Errors:\n")
		for _, err := range vr.ValidationErrors {
			report.WriteString(fmt.Sprintf("   %s: %s", err.Path, err.Message))
			if err.SchemaName != "" {
				report.WriteString(fmt.Sprintf(" (from %s)", err.SchemaName))
			}
			report.WriteString("\n")
		}
	}

	return report.String()
}

// HasErrors returns true if there were validation errors
func (vr *ValidationResult) HasErrors() bool {
	return len(vr.ValidationErrors) > 0
}

// GetErrorCount returns the total number of validation errors
func (vr *ValidationResult) GetErrorCount() int {
	return len(vr.ValidationErrors)
}

// GetErrorsForSchema returns validation errors for a specific schema
func (vr *ValidationResult) GetErrorsForSchema(schemaName string) []ValidationError {
	var errors []ValidationError
	for _, err := range vr.ValidationErrors {
		if err.SchemaName == schemaName {
			errors = append(errors, err)
		}
	}
	return errors
}

// NewValidationResult creates a new validation result with basic information
func NewValidationResult(intendedSchema string) *ValidationResult {
	return &ValidationResult{
		IntendedSchema:   intendedSchema,
		Success:          true, // Never reject data
		CascadeAttempts:  []CascadeAttempt{},
		ValidationErrors: []ValidationError{},
	}
}

// AddCascadeAttempt adds a validation attempt to the result
func (vr *ValidationResult) AddCascadeAttempt(schemaName string, success bool, errors []ValidationError, reason string) {
	attempt := CascadeAttempt{
		SchemaName: schemaName,
		Success:    success,
		Errors:     errors,
		Reason:     reason,
	}
	vr.CascadeAttempts = append(vr.CascadeAttempts, attempt)

	// Add errors to the main error list
	for _, err := range errors {
		err.SchemaName = schemaName
		vr.ValidationErrors = append(vr.ValidationErrors, err)
	}
}

// SetFinalAssignment sets the final schema assignment
func (vr *ValidationResult) SetFinalAssignment(assignedSchema, fallbackReason string) {
	vr.AssignedSchema = assignedSchema
	vr.Valid = (assignedSchema == vr.IntendedSchema)
	vr.FallbackReason = fallbackReason
}
