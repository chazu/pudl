package errors

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrorHandler interface allows different contexts to handle errors appropriately
type ErrorHandler interface {
	HandleError(err error) error
}

// CLIErrorHandler handles errors in command-line context
type CLIErrorHandler struct {
	// ExitOnError determines whether to call os.Exit() for non-recoverable errors
	ExitOnError bool
	// Verbose enables detailed error output
	Verbose bool
}

// NewCLIErrorHandler creates a new CLI error handler
func NewCLIErrorHandler(exitOnError bool) *CLIErrorHandler {
	return &CLIErrorHandler{
		ExitOnError: exitOnError,
		Verbose:     false,
	}
}

// HandleError handles errors in CLI context with appropriate formatting and exit codes
func (h *CLIErrorHandler) HandleError(err error) error {
	if err == nil {
		return nil
	}

	var pudlErr *PUDLError
	if !errors.As(err, &pudlErr) {
		// Handle non-PUDL errors
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if h.ExitOnError {
			os.Exit(1)
		}
		return err
	}

	// Format PUDL error for CLI display
	h.displayError(pudlErr)

	// Handle exit behavior
	if h.ExitOnError {
		os.Exit(pudlErr.GetExitCode())
	}

	return pudlErr
}

// displayError formats and displays a PUDL error
func (h *CLIErrorHandler) displayError(err *PUDLError) {
	// Main error message
	fmt.Fprintf(os.Stderr, "Error: %s\n", err.Message)

	// Show context in verbose mode
	if h.Verbose && len(err.Context) > 0 {
		fmt.Fprintf(os.Stderr, "\nContext:\n")
		for key, value := range err.Context {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", key, value)
		}
	}

	// Show suggestions if available
	if len(err.Suggestions) > 0 {
		fmt.Fprintf(os.Stderr, "\nSuggestions:\n")
		for _, suggestion := range err.Suggestions {
			fmt.Fprintf(os.Stderr, "  • %s\n", suggestion)
		}
	}

	// Show underlying cause in verbose mode
	if h.Verbose && err.Cause != nil {
		fmt.Fprintf(os.Stderr, "\nUnderlying cause: %v\n", err.Cause)
	}

	// Show error code in verbose mode
	if h.Verbose {
		fmt.Fprintf(os.Stderr, "\nError code: %s\n", err.Code)
	}
}

// SetVerbose enables or disables verbose error output
func (h *CLIErrorHandler) SetVerbose(verbose bool) {
	h.Verbose = verbose
}

// TUIErrorHandler handles errors in Terminal UI context (for future Bubble Tea integration)
type TUIErrorHandler struct {
	// ErrorChan sends errors to the TUI for display
	ErrorChan chan error
	// MaxErrors limits the number of errors queued
	MaxErrors int
}

// NewTUIErrorHandler creates a new TUI error handler
func NewTUIErrorHandler(maxErrors int) *TUIErrorHandler {
	return &TUIErrorHandler{
		ErrorChan: make(chan error, maxErrors),
		MaxErrors: maxErrors,
	}
}

// HandleError handles errors in TUI context by sending them to the error channel
func (h *TUIErrorHandler) HandleError(err error) error {
	if err == nil {
		return nil
	}

	// Send error to TUI for display, don't terminate the program
	select {
	case h.ErrorChan <- err:
		// Error queued successfully
	default:
		// Channel full, could log this or handle overflow
		// For now, just return the error
	}

	return err
}

// GetErrorChannel returns the error channel for TUI consumption
func (h *TUIErrorHandler) GetErrorChannel() <-chan error {
	return h.ErrorChan
}

// Close closes the error channel
func (h *TUIErrorHandler) Close() {
	close(h.ErrorChan)
}

// TestErrorHandler handles errors in test context (never exits, captures errors)
type TestErrorHandler struct {
	// Errors stores all handled errors for test verification
	Errors []error
	// Verbose enables detailed error output for debugging tests
	Verbose bool
}

// NewTestErrorHandler creates a new test error handler
func NewTestErrorHandler() *TestErrorHandler {
	return &TestErrorHandler{
		Errors:  make([]error, 0),
		Verbose: false,
	}
}

// HandleError handles errors in test context by capturing them
func (h *TestErrorHandler) HandleError(err error) error {
	if err == nil {
		return nil
	}

	h.Errors = append(h.Errors, err)

	if h.Verbose {
		var pudlErr *PUDLError
		if errors.As(err, &pudlErr) {
			fmt.Printf("Test Error: %s (Code: %s)\n", pudlErr.Message, pudlErr.Code)
		} else {
			fmt.Printf("Test Error: %v\n", err)
		}
	}

	return err
}

// GetErrors returns all captured errors
func (h *TestErrorHandler) GetErrors() []error {
	return h.Errors
}

// GetLastError returns the most recent error, or nil if no errors
func (h *TestErrorHandler) GetLastError() error {
	if len(h.Errors) == 0 {
		return nil
	}
	return h.Errors[len(h.Errors)-1]
}

// HasErrorCode checks if any captured error has the specified code
func (h *TestErrorHandler) HasErrorCode(code ErrorCode) bool {
	for _, err := range h.Errors {
		if GetErrorCode(err) == code {
			return true
		}
	}
	return false
}

// Clear clears all captured errors
func (h *TestErrorHandler) Clear() {
	h.Errors = h.Errors[:0]
}

// SetVerbose enables or disables verbose error output for tests
func (h *TestErrorHandler) SetVerbose(verbose bool) {
	h.Verbose = verbose
}

// FormatErrorForUser formats an error for user-friendly display
func FormatErrorForUser(err error) string {
	if err == nil {
		return ""
	}

	var pudlErr *PUDLError
	if !errors.As(err, &pudlErr) {
		return fmt.Sprintf("Error: %v", err)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Error: %s", pudlErr.Message))

	if len(pudlErr.Suggestions) > 0 {
		result.WriteString("\n\nSuggestions:")
		for _, suggestion := range pudlErr.Suggestions {
			result.WriteString(fmt.Sprintf("\n  • %s", suggestion))
		}
	}

	return result.String()
}

// FormatErrorForLogging formats an error for structured logging
func FormatErrorForLogging(err error) map[string]interface{} {
	if err == nil {
		return nil
	}

	var pudlErr *PUDLError
	if !errors.As(err, &pudlErr) {
		return map[string]interface{}{
			"error":   err.Error(),
			"type":    "unknown",
			"code":    "UNKNOWN",
			"message": err.Error(),
		}
	}

	logData := map[string]interface{}{
		"error":       pudlErr.Error(),
		"type":        "pudl_error",
		"code":        string(pudlErr.Code),
		"message":     pudlErr.Message,
		"recoverable": pudlErr.Recoverable,
	}

	if len(pudlErr.Context) > 0 {
		logData["context"] = pudlErr.Context
	}

	if len(pudlErr.Suggestions) > 0 {
		logData["suggestions"] = pudlErr.Suggestions
	}

	if pudlErr.Cause != nil {
		logData["cause"] = pudlErr.Cause.Error()
	}

	return logData
}
