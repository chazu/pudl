package review

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"pudl/internal/errors"
	"pudl/internal/schema"
)

//go:embed schema.cue.tmpl
var schemaTemplate string

// SchemaCreator handles creating new schemas from data
type SchemaCreator struct {
	schemaMgr *schema.Manager
	validator *schema.Validator
	schemaPath string
}

// NewSchemaCreator creates a new schema creator
func NewSchemaCreator(schemaMgr *schema.Manager, validator *schema.Validator, schemaPath string) *SchemaCreator {
	return &SchemaCreator{
		schemaMgr:  schemaMgr,
		validator:  validator,
		schemaPath: schemaPath,
	}
}

// CreateSchemaFromData creates a new CUE schema from the provided data
func (sc *SchemaCreator) CreateSchemaFromData(data interface{}, suggestedName string) (string, error) {
	// Generate CUE schema template from data
	cueTemplate, err := sc.generateCUETemplate(data, suggestedName)
	if err != nil {
		return "", err
	}

	// Create temporary file for editing
	tempFile, err := sc.createTempFile(cueTemplate)
	if err != nil {
		return "", err
	}
	defer os.Remove(tempFile)

	// Open editor for user customization
	if err := sc.openEditor(tempFile); err != nil {
		return "", err
	}

	// Read the edited content
	editedContent, err := ioutil.ReadFile(tempFile)
	if err != nil {
		return "", errors.NewFileNotFoundError(tempFile)
	}

	// Validate the edited schema
	result, err := sc.validator.ValidateSchemaContent(string(editedContent))
	if err != nil {
		return "", err
	}

	if !result.Valid {
		return "", errors.NewCUESyntaxError(tempFile, fmt.Errorf("validation failed: %s", strings.Join(result.Errors, "; ")))
	}

	// Extract package and schema name from the content
	packageName, schemaName, err := sc.extractSchemaInfo(string(editedContent))
	if err != nil {
		return "", err
	}

	// Save the schema to the appropriate location
	schemaPath, err := sc.saveSchema(packageName, schemaName, string(editedContent))
	if err != nil {
		return "", err
	}

	fullSchemaName := fmt.Sprintf("%s.%s", packageName, schemaName)
	fmt.Printf("✅ Schema created successfully: %s\n", fullSchemaName)
	fmt.Printf("📁 Saved to: %s\n", schemaPath)

	return fullSchemaName, nil
}

// schemaTemplateData holds data for the schema template
type schemaTemplateData struct {
	PackageName      string
	SchemaName       string
	SchemaNameLower  string
	SuggestedName    string
	IdentityFields   string
	TrackedFields    string
	FieldDefinitions string
	ExampleJSON      string
}

// generateCUETemplate generates a CUE schema template from JSON data
func (sc *SchemaCreator) generateCUETemplate(data interface{}, suggestedName string) (string, error) {
	// Convert data to map for analysis
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", errors.NewParsingError("JSON", err)
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &dataMap); err != nil {
		return "", errors.NewParsingError("JSON", err)
	}

	// Generate schema name and package
	packageName, schemaName := sc.generateSchemaNames(suggestedName, dataMap)

	// Generate field definitions
	fieldDefs := sc.generateFieldDefinitionsString(dataMap, 1)

	// Generate example JSON as line comments
	exampleJSON, _ := json.MarshalIndent(dataMap, "", "  ")
	exampleLines := strings.Split(string(exampleJSON), "\n")
	for i, line := range exampleLines {
		exampleLines[i] = "// " + line
	}
	commentedExample := strings.Join(exampleLines, "\n")

	// Prepare template data
	tmplData := schemaTemplateData{
		PackageName:      packageName,
		SchemaName:       schemaName,
		SchemaNameLower:  strings.ToLower(schemaName),
		SuggestedName:    suggestedName,
		IdentityFields:   sc.formatStringArray(sc.generateIdentityFields(dataMap)),
		TrackedFields:    sc.formatStringArray(sc.generateTrackedFields(dataMap)),
		FieldDefinitions: fieldDefs,
		ExampleJSON:      commentedExample,
	}

	// Parse and execute template
	tmpl, err := template.New("schema").Parse(schemaTemplate)
	if err != nil {
		return "", errors.NewSystemError("Failed to parse schema template", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, tmplData); err != nil {
		return "", errors.NewSystemError("Failed to execute schema template", err)
	}

	return buf.String(), nil
}

// generateSchemaNames creates appropriate package and schema names
func (sc *SchemaCreator) generateSchemaNames(suggestedName string, data map[string]interface{}) (string, string) {
	// Default package
	packageName := "custom"
	
	// Try to infer package from data patterns
	if sc.hasAWSFields(data) {
		packageName = "aws"
	} else if sc.hasK8sFields(data) {
		packageName = "k8s"
	}
	
	// Generate schema name
	schemaName := "CustomResource"
	if suggestedName != "" {
		schemaName = sc.toCamelCase(suggestedName)
	} else if kind, ok := data["kind"].(string); ok {
		schemaName = kind
	} else if resourceType, ok := data["resourceType"].(string); ok {
		schemaName = sc.toCamelCase(resourceType)
	}
	
	return packageName, schemaName
}

// hasAWSFields checks if data contains AWS-specific fields
func (sc *SchemaCreator) hasAWSFields(data map[string]interface{}) bool {
	awsFields := []string{"arn", "awsRegion", "resourceType", "accountId"}
	for _, field := range awsFields {
		if _, exists := data[field]; exists {
			return true
		}
	}
	return false
}

// hasK8sFields checks if data contains Kubernetes-specific fields
func (sc *SchemaCreator) hasK8sFields(data map[string]interface{}) bool {
	k8sFields := []string{"apiVersion", "kind", "metadata"}
	for _, field := range k8sFields {
		if _, exists := data[field]; exists {
			return true
		}
	}
	return false
}

// generateIdentityFields identifies fields that could serve as identity fields
func (sc *SchemaCreator) generateIdentityFields(data map[string]interface{}) []string {
	var identityFields []string
	
	// Common identity field patterns
	identityPatterns := []string{
		"id", "name", "arn", "resourceId", "instanceId",
		"metadata.name", "metadata.uid", "spec.name",
	}
	
	for _, pattern := range identityPatterns {
		if sc.hasNestedField(data, pattern) {
			identityFields = append(identityFields, pattern)
		}
	}
	
	// If no identity fields found, use first string field
	if len(identityFields) == 0 {
		for key, value := range data {
			if reflect.TypeOf(value).Kind() == reflect.String {
				identityFields = append(identityFields, key)
				break
			}
		}
	}
	
	return identityFields
}

// generateTrackedFields identifies fields that should be tracked for changes
func (sc *SchemaCreator) generateTrackedFields(data map[string]interface{}) []string {
	var trackedFields []string
	
	// Common tracked field patterns
	trackedPatterns := []string{
		"status", "state", "spec", "configuration", "tags",
		"metadata.labels", "metadata.annotations",
	}
	
	for _, pattern := range trackedPatterns {
		if sc.hasNestedField(data, pattern) {
			trackedFields = append(trackedFields, pattern)
		}
	}
	
	return trackedFields
}

// hasNestedField checks if a nested field exists in the data
func (sc *SchemaCreator) hasNestedField(data map[string]interface{}, fieldPath string) bool {
	parts := strings.Split(fieldPath, ".")
	current := data
	
	for _, part := range parts {
		if value, exists := current[part]; exists {
			if len(parts) == 1 {
				return true
			}
			if nextMap, ok := value.(map[string]interface{}); ok {
				current = nextMap
				parts = parts[1:]
			} else {
				return false
			}
		} else {
			return false
		}
	}
	
	return true
}

// generateFieldDefinitionsString generates CUE field definitions and returns as a string
func (sc *SchemaCreator) generateFieldDefinitionsString(data map[string]interface{}, indent int) string {
	var builder strings.Builder
	sc.generateFieldDefinitions(&builder, data, indent)
	return builder.String()
}

// generateFieldDefinitions generates CUE field definitions from data
func (sc *SchemaCreator) generateFieldDefinitions(builder *strings.Builder, data map[string]interface{}, indent int) {
	// Sort keys for consistent output
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	indentStr := strings.Repeat("\t", indent)

	for _, key := range keys {
		value := data[key]

		// Add comment with example value for complex types
		if sc.shouldAddComment(value) {
			exampleValue := sc.getExampleValue(value)
			builder.WriteString(fmt.Sprintf("%s// Example: %s\n", indentStr, exampleValue))
		}

		cueType := sc.inferCUEType(value)

		// Handle nested objects
		if nestedMap, ok := value.(map[string]interface{}); ok && len(nestedMap) > 0 {
			builder.WriteString(fmt.Sprintf("%s%s: {\n", indentStr, key))
			sc.generateFieldDefinitions(builder, nestedMap, indent+1)
			builder.WriteString(fmt.Sprintf("%s}\n", indentStr))
		} else {
			builder.WriteString(fmt.Sprintf("%s%s: %s\n", indentStr, key, cueType))
		}
	}
}

// shouldAddComment determines if a field should have an example comment
func (sc *SchemaCreator) shouldAddComment(value interface{}) bool {
	switch v := value.(type) {
	case string:
		return len(v) > 0 && len(v) < 100 // Add comment for non-empty, reasonable length strings
	case []interface{}:
		return len(v) > 0 // Add comment for non-empty arrays
	default:
		return false
	}
}

// getExampleValue returns a string representation of the value for comments
func (sc *SchemaCreator) getExampleValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		if len(v) > 50 {
			return fmt.Sprintf("\"%s...\"", v[:47])
		}
		return fmt.Sprintf("\"%s\"", v)
	case []interface{}:
		if len(v) > 0 {
			first := sc.getExampleValue(v[0])
			if len(v) == 1 {
				return fmt.Sprintf("[%s]", first)
			}
			return fmt.Sprintf("[%s, ...] (%d items)", first, len(v))
		}
		return "[]"
	default:
		jsonBytes, _ := json.Marshal(value)
		result := string(jsonBytes)
		if len(result) > 50 {
			return result[:47] + "..."
		}
		return result
	}
}

// inferCUEType infers the CUE type from a Go value
func (sc *SchemaCreator) inferCUEType(value interface{}) string {
	if value == nil {
		return "null | _"
	}
	
	switch v := value.(type) {
	case string:
		return "string"
	case int, int32, int64, float32, float64:
		return "number"
	case bool:
		return "bool"
	case []interface{}:
		if len(v) > 0 {
			elementType := sc.inferCUEType(v[0])
			return fmt.Sprintf("[...%s]", elementType)
		}
		return "[...]"
	case map[string]interface{}:
		return "{...}"
	default:
		return "_"
	}
}

// formatStringArray formats a string array for CUE
func (sc *SchemaCreator) formatStringArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	
	quoted := make([]string, len(arr))
	for i, s := range arr {
		quoted[i] = fmt.Sprintf("\"%s\"", s)
	}
	
	return fmt.Sprintf("[%s]", strings.Join(quoted, ", "))
}

// toCamelCase converts a string to CamelCase
func (sc *SchemaCreator) toCamelCase(s string) string {
	// Simple camel case conversion
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	
	return strings.Join(parts, "")
}

// createTempFile creates a temporary file with the schema content
func (sc *SchemaCreator) createTempFile(content string) (string, error) {
	tempFile, err := ioutil.TempFile("", "pudl-schema-*.cue")
	if err != nil {
		return "", errors.NewSystemError("Failed to create temporary file", err)
	}
	defer tempFile.Close()

	if _, err := tempFile.WriteString(content); err != nil {
		return "", errors.NewSystemError("Failed to write to temporary file", err)
	}
	
	return tempFile.Name(), nil
}

// openEditor opens the user's preferred editor
func (sc *SchemaCreator) openEditor(filename string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi" // Default to vi
	}
	
	cmd := exec.Command(editor, filename)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return errors.NewSystemError(fmt.Sprintf("Failed to run editor: %s", editor), err)
	}
	
	return nil
}

// extractSchemaInfo extracts package and schema name from CUE content
func (sc *SchemaCreator) extractSchemaInfo(content string) (string, string, error) {
	lines := strings.Split(content, "\n")
	
	var packageName, schemaName string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Extract package name
		if strings.HasPrefix(line, "package ") {
			packageName = strings.TrimSpace(strings.TrimPrefix(line, "package"))
		}
		
		// Extract schema name (first #Definition)
		if strings.Contains(line, "#") && strings.Contains(line, ":") && schemaName == "" {
			parts := strings.Split(line, ":")
			if len(parts) > 0 {
				defPart := strings.TrimSpace(parts[0])
				if strings.HasPrefix(defPart, "#") {
					schemaName = strings.TrimPrefix(defPart, "#")
				}
			}
		}
	}
	
	if packageName == "" {
		return "", "", errors.NewInputError(
			"No package declaration found in schema",
			"Add a 'package <name>' declaration at the top of the file",
		)
	}

	if schemaName == "" {
		return "", "", errors.NewInputError(
			"No schema definition found",
			"Add a schema definition like '#MySchema: { ... }'",
		)
	}
	
	return packageName, schemaName, nil
}

// saveSchema saves the schema to the appropriate package directory
func (sc *SchemaCreator) saveSchema(packageName, schemaName, content string) (string, error) {
	// Create package directory
	packageDir := filepath.Join(sc.schemaPath, packageName)
	if err := os.MkdirAll(packageDir, 0755); err != nil {
		return "", errors.NewSystemError("Failed to create package directory", err)
	}

	// Generate filename
	filename := fmt.Sprintf("%s.cue", strings.ToLower(schemaName))
	schemaPath := filepath.Join(packageDir, filename)

	// Check if file already exists
	if _, err := os.Stat(schemaPath); err == nil {
		return "", errors.NewInputError(
			fmt.Sprintf("Schema file already exists: %s", schemaPath),
			"Choose a different schema name or remove the existing file",
		)
	}

	// Write the schema file
	if err := ioutil.WriteFile(schemaPath, []byte(content), 0644); err != nil {
		return "", errors.NewSystemError("Failed to save schema file", err)
	}
	
	return schemaPath, nil
}
