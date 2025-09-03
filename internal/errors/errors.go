package errors

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrorCode represents the type of error for programmatic handling
type ErrorCode string

const (
	// User Input Errors
	ErrCodeInvalidInput     ErrorCode = "INVALID_INPUT"
	ErrCodeMissingRequired  ErrorCode = "MISSING_REQUIRED"
	ErrCodeFileNotFound     ErrorCode = "FILE_NOT_FOUND"
	ErrCodeInvalidFormat    ErrorCode = "INVALID_FORMAT"

	// System Errors
	ErrCodeFileSystem  ErrorCode = "FILE_SYSTEM"
	ErrCodePermission  ErrorCode = "PERMISSION"
	ErrCodeNetwork     ErrorCode = "NETWORK"

	// Configuration Errors
	ErrCodeConfigInvalid    ErrorCode = "CONFIG_INVALID"
	ErrCodeWorkspaceNotInit ErrorCode = "WORKSPACE_NOT_INIT"

	// Schema/Validation Errors
	ErrCodeSchemaNotFound   ErrorCode = "SCHEMA_NOT_FOUND"
	ErrCodeValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrCodeCUESyntax        ErrorCode = "CUE_SYNTAX"

	// Data Processing Errors
	ErrCodeParsingFailed ErrorCode = "PARSING_FAILED"
	ErrCodeMemoryLimit   ErrorCode = "MEMORY_LIMIT"
	ErrCodeTimeout       ErrorCode = "TIMEOUT"

	// Git/Repository Errors
	ErrCodeGitOperation ErrorCode = "GIT_OPERATION"
	ErrCodeRepoNotFound ErrorCode = "REPO_NOT_FOUND"
)

// PUDLError represents a structured error with context and recovery information
type PUDLError struct {
	Code        ErrorCode              `json:"code"`
	Message     string                 `json:"message"`
	Context     map[string]interface{} `json:"context,omitempty"`
	Suggestions []string               `json:"suggestions,omitempty"`
	Cause       error                  `json:"-"` // Original error
	Recoverable bool                   `json:"recoverable"`
}

// Error implements the error interface
func (e *PUDLError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap implements error unwrapping for errors.Unwrap()
func (e *PUDLError) Unwrap() error {
	return e.Cause
}

// Is implements error matching for errors.Is()
func (e *PUDLError) Is(target error) bool {
	if t, ok := target.(*PUDLError); ok {
		return e.Code == t.Code
	}
	return false
}

// MarshalJSON implements json.Marshaler for structured logging
func (e *PUDLError) MarshalJSON() ([]byte, error) {
	type Alias PUDLError
	aux := &struct {
		*Alias
		CauseMessage string `json:"cause,omitempty"`
	}{
		Alias: (*Alias)(e),
	}
	if e.Cause != nil {
		aux.CauseMessage = e.Cause.Error()
	}
	return json.Marshal(aux)
}

// GetExitCode returns an appropriate exit code for CLI usage
func (e *PUDLError) GetExitCode() int {
	switch e.Code {
	case ErrCodeInvalidInput, ErrCodeMissingRequired, ErrCodeFileNotFound, ErrCodeInvalidFormat:
		return 2 // Invalid usage
	case ErrCodePermission:
		return 77 // Permission denied (EX_NOPERM)
	case ErrCodeConfigInvalid, ErrCodeWorkspaceNotInit:
		return 78 // Configuration error (EX_CONFIG)
	case ErrCodeFileSystem, ErrCodeNetwork:
		return 74 // I/O error (EX_IOERR)
	default:
		return 1 // General error
	}
}

// NewInputError creates an error for invalid user input
func NewInputError(message string, suggestions ...string) *PUDLError {
	return &PUDLError{
		Code:        ErrCodeInvalidInput,
		Message:     message,
		Suggestions: suggestions,
		Recoverable: true,
	}
}

// NewMissingRequiredError creates an error for missing required parameters
func NewMissingRequiredError(parameter string, suggestions ...string) *PUDLError {
	return &PUDLError{
		Code:    ErrCodeMissingRequired,
		Message: fmt.Sprintf("Missing required parameter: %s", parameter),
		Context: map[string]interface{}{"parameter": parameter},
		Suggestions: append([]string{
			fmt.Sprintf("Provide the --%s flag", parameter),
		}, suggestions...),
		Recoverable: true,
	}
}

// NewFileNotFoundError creates an error for missing files
func NewFileNotFoundError(path string) *PUDLError {
	return &PUDLError{
		Code:    ErrCodeFileNotFound,
		Message: fmt.Sprintf("File not found: %s", path),
		Context: map[string]interface{}{"path": path},
		Suggestions: []string{
			"Check that the file path is correct",
			"Ensure the file exists and is readable",
			"Use an absolute path if the relative path is unclear",
		},
		Recoverable: true,
	}
}

// NewInvalidFormatError creates an error for unsupported file formats
func NewInvalidFormatError(format string, supportedFormats []string) *PUDLError {
	return &PUDLError{
		Code:    ErrCodeInvalidFormat,
		Message: fmt.Sprintf("Unsupported format: %s", format),
		Context: map[string]interface{}{
			"format":            format,
			"supported_formats": supportedFormats,
		},
		Suggestions: []string{
			fmt.Sprintf("Supported formats: %v", supportedFormats),
			"Convert the file to a supported format",
		},
		Recoverable: true,
	}
}

// NewConfigError creates an error for configuration issues
func NewConfigError(message string, cause error) *PUDLError {
	suggestions := []string{
		"Check your configuration with 'pudl config'",
		"Reset configuration with 'pudl config reset'",
	}
	if cause != nil && errors.Is(cause, fmt.Errorf("no such file or directory")) {
		suggestions = append(suggestions, "Initialize workspace with 'pudl init'")
	}

	return &PUDLError{
		Code:        ErrCodeConfigInvalid,
		Message:     message,
		Cause:       cause,
		Suggestions: suggestions,
		Recoverable: true,
	}
}

// NewWorkspaceNotInitError creates an error for uninitialized workspace
func NewWorkspaceNotInitError() *PUDLError {
	return &PUDLError{
		Code:    ErrCodeWorkspaceNotInit,
		Message: "PUDL workspace not initialized",
		Suggestions: []string{
			"Initialize workspace with 'pudl init'",
			"Check that you're in the correct directory",
		},
		Recoverable: true,
	}
}

// NewSchemaNotFoundError creates an error for missing schemas
func NewSchemaNotFoundError(schemaName string, availableSchemas []string) *PUDLError {
	suggestions := []string{
		"Check available schemas with 'pudl schema list'",
	}
	if len(availableSchemas) > 0 {
		suggestions = append(suggestions, fmt.Sprintf("Available schemas: %v", availableSchemas))
	}

	return &PUDLError{
		Code:    ErrCodeSchemaNotFound,
		Message: fmt.Sprintf("Schema not found: %s", schemaName),
		Context: map[string]interface{}{
			"schema":            schemaName,
			"available_schemas": availableSchemas,
		},
		Suggestions: suggestions,
		Recoverable: true,
	}
}

// NewValidationError creates an error for validation failures
func NewValidationError(schemaName string, details []string, cause error) *PUDLError {
	return &PUDLError{
		Code:    ErrCodeValidationFailed,
		Message: fmt.Sprintf("Validation failed for schema %s", schemaName),
		Context: map[string]interface{}{
			"schema":  schemaName,
			"details": details,
		},
		Suggestions: []string{
			"Check the data format against the schema requirements",
			fmt.Sprintf("Use 'pudl schema show %s' to view schema details", schemaName),
			"Consider using a different schema or fixing the data",
		},
		Cause:       cause,
		Recoverable: true,
	}
}

// NewCUESyntaxError creates an error for CUE syntax issues
func NewCUESyntaxError(filename string, cause error) *PUDLError {
	return &PUDLError{
		Code:    ErrCodeCUESyntax,
		Message: fmt.Sprintf("CUE syntax error in %s", filename),
		Context: map[string]interface{}{"filename": filename},
		Suggestions: []string{
			"Check the CUE syntax in the file",
			"Use 'cue fmt' to format the file",
			"Refer to CUE documentation for syntax help",
		},
		Cause:       cause,
		Recoverable: true,
	}
}

// NewParsingError creates an error for data parsing failures
func NewParsingError(format string, cause error) *PUDLError {
	return &PUDLError{
		Code:    ErrCodeParsingFailed,
		Message: fmt.Sprintf("Failed to parse %s data", format),
		Context: map[string]interface{}{"format": format},
		Suggestions: []string{
			"Check that the file is valid " + format,
			"Verify the file is not corrupted",
			"Try opening the file in a text editor to check format",
		},
		Cause:       cause,
		Recoverable: false,
	}
}

// NewGitError creates an error for git operations
func NewGitError(operation string, cause error) *PUDLError {
	return &PUDLError{
		Code:    ErrCodeGitOperation,
		Message: fmt.Sprintf("Git operation failed: %s", operation),
		Context: map[string]interface{}{"operation": operation},
		Suggestions: []string{
			"Check that git is installed and available",
			"Ensure the repository is properly initialized",
			"Check git status and resolve any conflicts",
		},
		Cause:       cause,
		Recoverable: true,
	}
}

// WrapError wraps an existing error with PUDL context
func WrapError(code ErrorCode, message string, cause error) *PUDLError {
	return &PUDLError{
		Code:        code,
		Message:     message,
		Cause:       cause,
		Recoverable: false, // Default to non-recoverable for wrapped errors
	}
}

// IsRecoverable checks if an error is recoverable
func IsRecoverable(err error) bool {
	var pudlErr *PUDLError
	if errors.As(err, &pudlErr) {
		return pudlErr.Recoverable
	}
	return false
}

// GetErrorCode extracts the error code from an error
func GetErrorCode(err error) ErrorCode {
	var pudlErr *PUDLError
	if errors.As(err, &pudlErr) {
		return pudlErr.Code
	}
	return ""
}
