package schema

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
)

// Validator handles CUE schema validation
type Validator struct {
	ctx *cue.Context
}

// NewValidator creates a new schema validator
func NewValidator() *Validator {
	return &Validator{
		ctx: cuecontext.New(),
	}
}

// ValidationResult represents the result of schema validation
type ValidationResult struct {
	Valid       bool     `json:"valid"`
	Errors      []string `json:"errors,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	PackageName string   `json:"package_name,omitempty"`
	Definitions []string `json:"definitions,omitempty"`
}

// ValidateSchema performs comprehensive validation of a CUE schema file
func (v *Validator) ValidateSchema(filePath string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}

	// Parse the CUE file
	file, err := parser.ParseFile(filePath, content, parser.ParseComments)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("CUE syntax error: %v", err))
		return result, nil
	}

	// Build CUE value
	value := v.ctx.BuildFile(file)
	if err := value.Err(); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("CUE build error: %v", err))
		return result, nil
	}

	// Validate CUE value
	if err := value.Validate(); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("CUE validation error: %v", err))
		return result, nil
	}

	// Extract package name
	if packageName := v.extractPackageName(string(content)); packageName != "" {
		result.PackageName = packageName
	} else {
		result.Warnings = append(result.Warnings, "No package declaration found")
	}

	// Extract definitions
	definitions := v.extractDefinitions(string(content))
	result.Definitions = definitions

	if len(definitions) == 0 {
		result.Warnings = append(result.Warnings, "No schema definitions found (no #Definition patterns)")
	}

	return result, nil
}

// ValidateSchemaContent validates CUE content directly (for testing)
func (v *Validator) ValidateSchemaContent(content string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Parse the CUE content
	file, err := parser.ParseFile("", content, parser.ParseComments)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("CUE syntax error: %v", err))
		return result, nil
	}

	// Build CUE value
	value := v.ctx.BuildFile(file)
	if err := value.Err(); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("CUE build error: %v", err))
		return result, nil
	}

	// Validate CUE value
	if err := value.Validate(); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("CUE validation error: %v", err))
		return result, nil
	}

	// Extract package name
	if packageName := v.extractPackageName(content); packageName != "" {
		result.PackageName = packageName
	}

	// Extract definitions
	definitions := v.extractDefinitions(content)
	result.Definitions = definitions

	return result, nil
}

// extractPackageName extracts the package name from CUE content
func (v *Validator) extractPackageName(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

// extractDefinitions extracts schema definition names from CUE content
func (v *Validator) extractDefinitions(content string) []string {
	var definitions []string

	// Regular expression to match #DefinitionName: patterns
	re := regexp.MustCompile(`#([A-Za-z][A-Za-z0-9_]*)\s*:`)
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 1 {
			definitions = append(definitions, "#"+match[1])
		}
	}

	return definitions
}

// ValidatePackageConsistency validates that a schema file's package matches its intended package
func (v *Validator) ValidatePackageConsistency(filePath, expectedPackage string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	actualPackage := v.extractPackageName(string(content))
	if actualPackage == "" {
		return fmt.Errorf("no package declaration found in schema file")
	}

	if actualPackage != expectedPackage {
		return fmt.Errorf("package mismatch: file declares package '%s' but expected '%s'", actualPackage, expectedPackage)
	}

	return nil
}
