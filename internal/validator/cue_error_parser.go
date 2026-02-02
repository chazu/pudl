package validator

import (
	"regexp"
	"strings"
)

// ParsedError represents a parsed CUE validation error
type ParsedError struct {
	Path        string // CUE path where error occurred
	Expected    string // Expected value/type
	Got         string // Actual value/type
	Constraint  string // The constraint that was violated
	Suggestion  string // Suggested fix
}

// CUEErrorParser parses CUE validation errors into structured format
type CUEErrorParser struct {
	patterns []*errorPattern
}

type errorPattern struct {
	name    string
	regex   *regexp.Regexp
	extract func(matches []string) *ParsedError
}

// NewCUEErrorParser creates a new CUE error parser
func NewCUEErrorParser() *CUEErrorParser {
	parser := &CUEErrorParser{}
	parser.initializePatterns()
	return parser
}

// initializePatterns sets up regex patterns for common CUE errors
func (p *CUEErrorParser) initializePatterns() {
	p.patterns = []*errorPattern{
		{
			name:  "conflicting_values",
			regex: regexp.MustCompile(`^(.+?):\s+conflicting values\s+(.+?)\s+and\s+(.+?)(?:\s+\(.*?\))?$`),
			extract: func(matches []string) *ParsedError {
				return &ParsedError{
					Path:       matches[1],
					Expected:   matches[2],
					Got:        matches[3],
					Constraint: "conflicting values",
					Suggestion: "Ensure field has a single consistent value",
				}
			},
		},
		{
			name:  "incomplete_value",
			regex: regexp.MustCompile(`^(.+?):\s+incomplete value\s+(.+?)$`),
			extract: func(matches []string) *ParsedError {
				return &ParsedError{
					Path:       matches[1],
					Constraint: "incomplete value",
					Got:        matches[2],
					Suggestion: "Provide a complete value for this field",
				}
			},
		},
		{
			name:  "value_not_allowed",
			regex: regexp.MustCompile(`^(.+?):\s+value\s+(.+?)\s+not allowed\s+\(.*?:\s+(.+?)\)$`),
			extract: func(matches []string) *ParsedError {
				return &ParsedError{
					Path:       matches[1],
					Got:        matches[2],
					Expected:   matches[3],
					Constraint: "value not allowed",
					Suggestion: "Use one of the allowed values",
				}
			},
		},
		{
			name:  "type_mismatch",
			regex: regexp.MustCompile(`^(.+?):\s+(.+?)\s+\(type\s+(.+?)\)\s+does not match\s+(.+?)\s+\(type\s+(.+?)\)$`),
			extract: func(matches []string) *ParsedError {
				return &ParsedError{
					Path:       matches[1],
					Got:        matches[2] + " (type " + matches[3] + ")",
					Expected:   matches[4] + " (type " + matches[5] + ")",
					Constraint: "type mismatch",
					Suggestion: "Ensure the value type matches the schema definition",
				}
			},
		},
		{
			name:  "missing_field",
			regex: regexp.MustCompile(`^(.+?):\s+missing required field\s+(.+?)$`),
			extract: func(matches []string) *ParsedError {
				return &ParsedError{
					Path:       matches[1],
					Expected:   matches[2],
					Constraint: "missing required field",
					Suggestion: "Add the required field to your data",
				}
			},
		},
		{
			name:  "generic_error",
			regex: regexp.MustCompile(`^(.+?):\s+(.+?)$`),
			extract: func(matches []string) *ParsedError {
				return &ParsedError{
					Path:       matches[1],
					Constraint: matches[2],
					Suggestion: "Review the constraint and adjust your data accordingly",
				}
			},
		},
	}
}

// Parse parses a CUE error into structured ParsedError objects
func (p *CUEErrorParser) Parse(err error) []ParsedError {
	if err == nil {
		return nil
	}

	var results []ParsedError
	errorMsg := err.Error()

	// Split by newlines to handle multiple errors
	lines := strings.Split(errorMsg, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try each pattern
		for _, pattern := range p.patterns {
			matches := pattern.regex.FindStringSubmatch(line)
			if len(matches) > 0 {
				parsed := pattern.extract(matches)
				if parsed != nil {
					results = append(results, *parsed)
				}
				break
			}
		}
	}

	return results
}

// FormatError formats a ParsedError into a human-readable string
func (p *CUEErrorParser) FormatError(pe ParsedError) string {
	var parts []string

	parts = append(parts, "Path: "+pe.Path)

	if pe.Expected != "" {
		parts = append(parts, "Expected: "+pe.Expected)
	}

	if pe.Got != "" {
		parts = append(parts, "Got: "+pe.Got)
	}

	if pe.Constraint != "" {
		parts = append(parts, "Constraint: "+pe.Constraint)
	}

	if pe.Suggestion != "" {
		parts = append(parts, "Suggestion: "+pe.Suggestion)
	}

	return strings.Join(parts, " | ")
}

