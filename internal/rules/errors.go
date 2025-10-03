package rules

import (
	"fmt"
)

// ErrorCode represents specific rule engine error types
type ErrorCode string

const (
	// Engine management errors
	ErrorCodeUnknownEngine     ErrorCode = "UNKNOWN_ENGINE"
	ErrorCodeInitializationFailed ErrorCode = "INITIALIZATION_FAILED"
	ErrorCodeConfigurationError   ErrorCode = "CONFIGURATION_ERROR"

	// Rule loading errors
	ErrorCodeRuleLoadFailed    ErrorCode = "RULE_LOAD_FAILED"
	ErrorCodeRuleCompileFailed ErrorCode = "RULE_COMPILE_FAILED"
	ErrorCodeRuleNotFound      ErrorCode = "RULE_NOT_FOUND"

	// Execution errors
	ErrorCodeExecutionTimeout  ErrorCode = "EXECUTION_TIMEOUT"
	ErrorCodeExecutionFailed   ErrorCode = "EXECUTION_FAILED"
	ErrorCodeInvalidInput      ErrorCode = "INVALID_INPUT"
	ErrorCodeMemoryLimitExceeded ErrorCode = "MEMORY_LIMIT_EXCEEDED"

	// Result errors
	ErrorCodeInvalidResult     ErrorCode = "INVALID_RESULT"
	ErrorCodeNoRuleMatched     ErrorCode = "NO_RULE_MATCHED"
)

// RuleEngineError represents an error from the rule engine system
type RuleEngineError struct {
	Code     ErrorCode              `json:"code"`
	Message  string                 `json:"message"`
	Context  map[string]interface{} `json:"context,omitempty"`
	Cause    error                  `json:"-"`
}

// Error implements the error interface
func (e *RuleEngineError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause
func (e *RuleEngineError) Unwrap() error {
	return e.Cause
}

// Is checks if the error matches a specific error code
func (e *RuleEngineError) Is(target error) bool {
	if targetErr, ok := target.(*RuleEngineError); ok {
		return e.Code == targetErr.Code
	}
	return false
}

// NewRuleEngineError creates a new rule engine error
func NewRuleEngineError(code ErrorCode, message string) *RuleEngineError {
	return &RuleEngineError{
		Code:    code,
		Message: message,
		Context: make(map[string]interface{}),
	}
}

// NewRuleEngineErrorWithCause creates a new rule engine error with a cause
func NewRuleEngineErrorWithCause(code ErrorCode, message string, cause error) *RuleEngineError {
	return &RuleEngineError{
		Code:    code,
		Message: message,
		Context: make(map[string]interface{}),
		Cause:   cause,
	}
}

// WithContext adds context information to the error
func (e *RuleEngineError) WithContext(key string, value interface{}) *RuleEngineError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// Common error constructors for convenience

// ErrUnknownEngine creates an error for unknown engine types
func ErrUnknownEngine(engineType string) *RuleEngineError {
	return NewRuleEngineError(ErrorCodeUnknownEngine, "unknown rule engine type").
		WithContext("engine_type", engineType)
}

// ErrInitializationFailed creates an error for initialization failures
func ErrInitializationFailed(cause error) *RuleEngineError {
	return NewRuleEngineErrorWithCause(ErrorCodeInitializationFailed, "rule engine initialization failed", cause)
}

// ErrConfigurationError creates an error for configuration issues
func ErrConfigurationError(message string) *RuleEngineError {
	return NewRuleEngineError(ErrorCodeConfigurationError, message)
}

// ErrRuleLoadFailed creates an error for rule loading failures
func ErrRuleLoadFailed(ruleName string, cause error) *RuleEngineError {
	return NewRuleEngineErrorWithCause(ErrorCodeRuleLoadFailed, "failed to load rule", cause).
		WithContext("rule_name", ruleName)
}

// ErrExecutionTimeout creates an error for execution timeouts
func ErrExecutionTimeout(timeout int) *RuleEngineError {
	return NewRuleEngineError(ErrorCodeExecutionTimeout, "rule execution timed out").
		WithContext("timeout_ms", timeout)
}

// ErrInvalidInput creates an error for invalid input data
func ErrInvalidInput(message string) *RuleEngineError {
	return NewRuleEngineError(ErrorCodeInvalidInput, message)
}

// ErrNoRuleMatched creates an error when no rules match the input
func ErrNoRuleMatched() *RuleEngineError {
	return NewRuleEngineError(ErrorCodeNoRuleMatched, "no rule matched the input data")
}
