package validator

import (
	"fmt"
	"strings"
)

// ValidationResult represents the result of cascading schema validation
type ValidationResult struct {
	IntendedSchema   string            `json:"intended_schema"`   // What user requested
	AssignedSchema   string            `json:"assigned_schema"`   // What data actually got assigned to
	CascadeLevel     string            `json:"cascade_level"`     // "exact", "fallback", "catchall"
	ComplianceStatus string            `json:"compliance_status"` // "compliant", "non-compliant", "unknown"
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
	FallbackReason   string            `json:"fallback_reason,omitempty"`
	Success          bool              `json:"success"`           // Always true (never reject data)
	CascadeAttempts  []CascadeAttempt  `json:"cascade_attempts"`  // All validation attempts
}

// ValidationError represents a specific validation failure
type ValidationError struct {
	Path        string      `json:"path"`         // CUE path where error occurred
	Message     string      `json:"message"`      // Human-readable error message
	Value       interface{} `json:"value"`        // The value that caused the error
	SchemaName  string      `json:"schema_name"`  // Which schema produced this error
	Constraint  string      `json:"constraint"`   // The constraint that was violated
}

// CascadeAttempt represents a single validation attempt in the cascade
type CascadeAttempt struct {
	SchemaName string            `json:"schema_name"`
	Success    bool              `json:"success"`
	Errors     []ValidationError `json:"errors,omitempty"`
	Reason     string            `json:"reason,omitempty"`
}

// SchemaMetadata represents PUDL metadata embedded in CUE schemas
type SchemaMetadata struct {
	SchemaType       string   `json:"schema_type"`        // "base", "policy", "custom"
	ResourceType     string   `json:"resource_type"`      // "aws.ec2.instance", "k8s.pod"
	BaseSchema       string   `json:"base_schema"`        // Parent schema reference
	CascadePriority  int      `json:"cascade_priority"`   // Higher = more specific
	CascadeFallback  []string `json:"cascade_fallback"`   // Explicit fallback chain
	IdentityFields   []string `json:"identity_fields"`    // Fields that identify the resource
	TrackedFields    []string `json:"tracked_fields"`     // Fields to monitor for changes
	ComplianceLevel  string   `json:"compliance_level"`   // "strict", "moderate", "permissive"
	IsListType       bool     `json:"is_list_type"`       // True if schema is structurally a list/array type (derived from CUE, not metadata)
}

// GetComplianceStatus determines compliance status based on cascade result
func (vr *ValidationResult) GetComplianceStatus() string {
	if vr.CascadeLevel == "exact" {
		return "compliant"
	}
	
	if vr.CascadeLevel == "fallback" {
		// Check if we fell back from a policy schema to a base schema
		if strings.Contains(vr.IntendedSchema, "Compliant") || 
		   strings.Contains(vr.IntendedSchema, "Policy") {
			return "non-compliant"
		}
		return "partial"
	}
	
	if vr.CascadeLevel == "catchall" {
		return "unknown"
	}
	
	return "unknown"
}

// GetSummary returns a human-readable summary of the validation result
func (vr *ValidationResult) GetSummary() string {
	status := vr.GetComplianceStatus()
	
	switch vr.CascadeLevel {
	case "exact":
		return fmt.Sprintf("✅ Validated successfully against %s", vr.AssignedSchema)
	case "fallback":
		return fmt.Sprintf("⚠️  Fell back to %s (intended: %s) - Status: %s", 
			vr.AssignedSchema, vr.IntendedSchema, strings.ToUpper(status))
	case "catchall":
		return fmt.Sprintf("🔄 Assigned to catchall schema (intended: %s) - Status: %s", 
			vr.IntendedSchema, strings.ToUpper(status))
	default:
		return fmt.Sprintf("❓ Unknown validation result for %s", vr.IntendedSchema)
	}
}

// GetDetailedReport returns a detailed validation report
func (vr *ValidationResult) GetDetailedReport() string {
	var report strings.Builder
	
	report.WriteString(fmt.Sprintf("🎯 Intended Schema: %s\n", vr.IntendedSchema))
	report.WriteString(fmt.Sprintf("📋 Assigned Schema: %s\n", vr.AssignedSchema))
	report.WriteString(fmt.Sprintf("📊 Cascade Level: %s\n", strings.ToUpper(vr.CascadeLevel)))
	report.WriteString(fmt.Sprintf("⚖️  Compliance Status: %s\n", strings.ToUpper(vr.GetComplianceStatus())))
	
	if vr.FallbackReason != "" {
		report.WriteString(fmt.Sprintf("💭 Fallback Reason: %s\n", vr.FallbackReason))
	}
	
	if len(vr.CascadeAttempts) > 1 {
		report.WriteString("\n🔄 Cascade Attempts:\n")
		for i, attempt := range vr.CascadeAttempts {
			status := "❌"
			if attempt.Success {
				status = "✅"
			}
			report.WriteString(fmt.Sprintf("   %d. %s %s", i+1, status, attempt.SchemaName))
			if attempt.Reason != "" {
				report.WriteString(fmt.Sprintf(" (%s)", attempt.Reason))
			}
			report.WriteString("\n")
		}
	}
	
	if len(vr.ValidationErrors) > 0 {
		report.WriteString("\n❌ Validation Errors:\n")
		for _, err := range vr.ValidationErrors {
			report.WriteString(fmt.Sprintf("   • %s: %s", err.Path, err.Message))
			if err.SchemaName != "" {
				report.WriteString(fmt.Sprintf(" (from %s)", err.SchemaName))
			}
			report.WriteString("\n")
		}
	}
	
	return report.String()
}

// HasErrors returns true if there were validation errors during cascade
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

// IsCompliant returns true if the data meets the intended schema requirements
func (vr *ValidationResult) IsCompliant() bool {
	return vr.GetComplianceStatus() == "compliant"
}

// IsNonCompliant returns true if the data is a valid type but doesn't meet business rules
func (vr *ValidationResult) IsNonCompliant() bool {
	return vr.GetComplianceStatus() == "non-compliant"
}

// IsUnknown returns true if the data couldn't be classified properly
func (vr *ValidationResult) IsUnknown() bool {
	return vr.GetComplianceStatus() == "unknown"
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

// AddCascadeAttempt adds a cascade attempt to the result
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
		err.SchemaName = schemaName // Ensure schema name is set
		vr.ValidationErrors = append(vr.ValidationErrors, err)
	}
}

// SetFinalAssignment sets the final schema assignment and cascade level
func (vr *ValidationResult) SetFinalAssignment(assignedSchema, cascadeLevel, fallbackReason string) {
	vr.AssignedSchema = assignedSchema
	vr.CascadeLevel = cascadeLevel
	vr.FallbackReason = fallbackReason
	vr.ComplianceStatus = vr.GetComplianceStatus()
}
