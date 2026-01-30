package schemagen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Generator handles schema generation from data.
type Generator struct {
	SchemaPath string
}

// NewGenerator creates a new schema generator.
func NewGenerator(schemaPath string) *Generator {
	return &Generator{SchemaPath: schemaPath}
}

// GenerateOptions configures schema generation.
type GenerateOptions struct {
	FromID         string            // pudl ID to generate from
	PackagePath    string            // e.g., "aws/ec2"
	DefinitionName string            // e.g., "Instance" (without #)
	IsCollection   bool              // whether to create a collection schema
	InferHints     map[string]string // field hints like {"State": "enum"}
}

// GenerateResult contains the result of schema generation.
type GenerateResult struct {
	FilePath              string   // where the schema was created
	PackageName           string   // the CUE package name
	DefinitionName        string   // the definition created
	FieldCount            int      // number of fields inferred
	InferredIdentityFields []string // fields that look like identity fields
	Content               string   // generated CUE content
}

// FieldInfo holds analyzed field information.
type FieldInfo struct {
	Name       string
	CUEType    string
	Required   bool
	IsNullable bool
	EnumValues []string
	Nested     map[string]*FieldInfo
}

// FieldAnalysis holds the complete analysis of data structure.
type FieldAnalysis struct {
	Fields         map[string]*FieldInfo
	IdentityFields []string
	SampleCount    int
}

// Generate analyzes data and generates a CUE schema.
func (g *Generator) Generate(data interface{}, opts GenerateOptions) (*GenerateResult, error) {
	analysis := g.analyzeData(data, opts)

	content := g.generateCUEContent(analysis, opts)

	packageName := filepath.Base(opts.PackagePath)
	fileName := strings.ToLower(opts.DefinitionName) + ".cue"
	filePath := filepath.Join(g.SchemaPath, opts.PackagePath, fileName)

	return &GenerateResult{
		FilePath:              filePath,
		PackageName:           packageName,
		DefinitionName:        opts.DefinitionName,
		FieldCount:            len(analysis.Fields),
		InferredIdentityFields: analysis.IdentityFields,
		Content:               content,
	}, nil
}

// analyzeData analyzes the data structure and infers types.
func (g *Generator) analyzeData(data interface{}, opts GenerateOptions) *FieldAnalysis {
	analysis := &FieldAnalysis{
		Fields:         make(map[string]*FieldInfo),
		IdentityFields: []string{},
	}

	// Handle arrays (collections) - merge field info from all items
	if arr, ok := data.([]interface{}); ok && len(arr) > 0 {
		analysis.SampleCount = len(arr)
		fieldPresence := make(map[string]int)

		for _, item := range arr {
			if obj, ok := item.(map[string]interface{}); ok {
				g.mergeObjectFields(analysis.Fields, obj, opts, fieldPresence)
			}
		}
		// Mark optional fields (not present in all items)
		for name, count := range fieldPresence {
			if count < len(arr) {
				if fi, exists := analysis.Fields[name]; exists {
					fi.Required = false
				}
			}
		}
	} else if obj, ok := data.(map[string]interface{}); ok {
		analysis.SampleCount = 1
		fieldPresence := make(map[string]int)
		g.mergeObjectFields(analysis.Fields, obj, opts, fieldPresence)
	}

	// Identify likely identity fields
	for name := range analysis.Fields {
		if g.isLikelyIdentityField(name) {
			analysis.IdentityFields = append(analysis.IdentityFields, name)
		}
	}
	sort.Strings(analysis.IdentityFields)

	return analysis
}

// mergeObjectFields merges fields from an object into the analysis.
func (g *Generator) mergeObjectFields(fields map[string]*FieldInfo, obj map[string]interface{}, opts GenerateOptions, presence map[string]int) {
	for key, value := range obj {
		presence[key]++

		existing, exists := fields[key]
		inferred := g.inferFieldInfo(key, value, opts)

		if !exists {
			inferred.Required = true
			fields[key] = inferred
		} else {
			// Merge type information
			g.mergeFieldInfo(existing, inferred)
		}
	}
}

// inferFieldInfo infers field information from a value.
func (g *Generator) inferFieldInfo(name string, value interface{}, opts GenerateOptions) *FieldInfo {
	fi := &FieldInfo{Name: name, Required: true}

	// Check for enum hint
	if hint, ok := opts.InferHints[name]; ok && hint == "enum" {
		fi.EnumValues = []string{}
		if strVal, ok := value.(string); ok {
			fi.EnumValues = append(fi.EnumValues, strVal)
		}
		fi.CUEType = "enum"
		return fi
	}

	fi.CUEType, fi.IsNullable, fi.Nested = g.inferType(value, opts)
	return fi
}

// inferType infers the CUE type from a value.
func (g *Generator) inferType(value interface{}, opts GenerateOptions) (cueType string, nullable bool, nested map[string]*FieldInfo) {
	if value == nil {
		return "_", true, nil
	}

	switch v := value.(type) {
	case string:
		return "string", false, nil
	case bool:
		return "bool", false, nil
	case float64:
		// Check if it's a whole number
		if v == float64(int64(v)) {
			return "int", false, nil
		}
		return "number", false, nil
	case json.Number:
		if _, err := v.Int64(); err == nil {
			return "int", false, nil
		}
		return "number", false, nil
	case []interface{}:
		if len(v) > 0 {
			elemType, _, elemNested := g.inferType(v[0], opts)
			if elemNested != nil {
				return "object_array", false, elemNested
			}
			return fmt.Sprintf("[...%s]", elemType), false, nil
		}
		return "[...]", false, nil
	case map[string]interface{}:
		nested = make(map[string]*FieldInfo)
		for k, val := range v {
			nested[k] = g.inferFieldInfo(k, val, opts)
		}
		return "struct", false, nested
	default:
		return "_", false, nil
	}
}

// mergeFieldInfo merges new field info into existing.
func (g *Generator) mergeFieldInfo(existing, new *FieldInfo) {
	// Handle nullable merging
	if new.IsNullable || new.CUEType == "_" {
		existing.IsNullable = true
	}

	// Merge enum values
	if existing.CUEType == "enum" && new.CUEType == "enum" {
		for _, v := range new.EnumValues {
			if !containsString(existing.EnumValues, v) {
				existing.EnumValues = append(existing.EnumValues, v)
			}
		}
	}
}

// isLikelyIdentityField checks if a field name suggests an identity field.
func (g *Generator) isLikelyIdentityField(name string) bool {
	lower := strings.ToLower(name)
	if lower == "name" || lower == "arn" || lower == "id" {
		return true
	}
	if strings.HasSuffix(lower, "id") || strings.HasSuffix(name, "Id") || strings.HasSuffix(name, "ID") {
		return true
	}
	return false
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}


// generateCUEContent produces valid CUE schema content.
func (g *Generator) generateCUEContent(analysis *FieldAnalysis, opts GenerateOptions) string {
	var b strings.Builder
	packageName := filepath.Base(opts.PackagePath)

	b.WriteString(fmt.Sprintf("package %s\n\n", packageName))
	b.WriteString(fmt.Sprintf("#%s: {\n", opts.DefinitionName))

	// Write _pudl metadata
	b.WriteString("\t_pudl: {\n")
	b.WriteString(fmt.Sprintf("\t\tschema_type: \"%s\"\n", g.schemaType(opts)))
	b.WriteString(fmt.Sprintf("\t\tresource_type: \"%s.%s\"\n", packageName, strings.ToLower(opts.DefinitionName)))
	b.WriteString("\t\tcascade_priority: 100\n")
	b.WriteString("\t\tcascade_fallback: [\"pudl.unknown.#CatchAll\"]\n")
	b.WriteString(fmt.Sprintf("\t\tidentity_fields: %s\n", g.formatStringSlice(analysis.IdentityFields)))
	b.WriteString(fmt.Sprintf("\t\ttracked_fields: %s\n", g.formatTrackedFields(analysis)))
	b.WriteString("\t\tcompliance_level: \"strict\"\n")
	b.WriteString("\t}\n\n")

	// Write field definitions
	g.writeFields(&b, analysis.Fields, 1)

	b.WriteString("}\n")

	return b.String()
}

// writeFields writes field definitions to the builder.
func (g *Generator) writeFields(b *strings.Builder, fields map[string]*FieldInfo, indent int) {
	// Sort field names for deterministic output
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)

	indentStr := strings.Repeat("\t", indent)

	for _, name := range names {
		fi := fields[name]
		optional := ""
		if !fi.Required {
			optional = "?"
		}

		switch {
		case fi.CUEType == "enum" && len(fi.EnumValues) > 0:
			sort.Strings(fi.EnumValues)
			quoted := make([]string, len(fi.EnumValues))
			for i, v := range fi.EnumValues {
				quoted[i] = fmt.Sprintf("\"%s\"", v)
			}
			b.WriteString(fmt.Sprintf("%s%s%s: %s\n", indentStr, name, optional, strings.Join(quoted, " | ")))

		case fi.CUEType == "struct" && fi.Nested != nil:
			b.WriteString(fmt.Sprintf("%s%s%s: {\n", indentStr, name, optional))
			g.writeFields(b, fi.Nested, indent+1)
			b.WriteString(fmt.Sprintf("%s}\n", indentStr))

		case fi.CUEType == "object_array" && fi.Nested != nil:
			b.WriteString(fmt.Sprintf("%s%s%s: [...{\n", indentStr, name, optional))
			g.writeFields(b, fi.Nested, indent+1)
			b.WriteString(fmt.Sprintf("%s}]\n", indentStr))

		default:
			typeStr := fi.CUEType
			if fi.IsNullable && typeStr != "_" {
				typeStr = fmt.Sprintf("null | %s", typeStr)
			}
			b.WriteString(fmt.Sprintf("%s%s%s: %s\n", indentStr, name, optional, typeStr))
		}
	}
}

// schemaType returns the schema type based on options.
func (g *Generator) schemaType(opts GenerateOptions) string {
	if opts.IsCollection {
		return "collection"
	}
	return "base"
}

// formatStringSlice formats a string slice as CUE array.
func (g *Generator) formatStringSlice(slice []string) string {
	if len(slice) == 0 {
		return "[]"
	}
	quoted := make([]string, len(slice))
	for i, s := range slice {
		quoted[i] = fmt.Sprintf("\"%s\"", s)
	}
	return fmt.Sprintf("[%s]", strings.Join(quoted, ", "))
}

// formatTrackedFields returns tracked fields (all non-identity fields).
func (g *Generator) formatTrackedFields(analysis *FieldAnalysis) string {
	tracked := make([]string, 0)
	identitySet := make(map[string]bool)
	for _, f := range analysis.IdentityFields {
		identitySet[f] = true
	}
	for name := range analysis.Fields {
		if !identitySet[name] {
			tracked = append(tracked, name)
		}
	}
	sort.Strings(tracked)
	return g.formatStringSlice(tracked)
}

// WriteSchema writes the generated schema to the schema repository.
func (g *Generator) WriteSchema(result *GenerateResult, content string) error {
	// Create package directory
	dir := filepath.Dir(result.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create package directory: %w", err)
	}

	// Check if file already exists
	if _, err := os.Stat(result.FilePath); err == nil {
		return fmt.Errorf("schema file already exists: %s", result.FilePath)
	}

	// Write the file
	if err := os.WriteFile(result.FilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write schema file: %w", err)
	}

	return nil
}

// Helper to ensure valid CUE identifier
func sanitizeIdentifier(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i == 0 && unicode.IsDigit(r) {
			result.WriteRune('_')
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}