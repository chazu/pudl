package schemagen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"

	"pudl/internal/inference"
	"pudl/internal/typepattern"
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

// CollectionGenerateOptions configures smart collection schema generation.
type CollectionGenerateOptions struct {
	PackagePath          string            // e.g., "aws/ec2"
	CollectionName       string            // e.g., "Ec2InstanceCollection" (without #)
	InferHints           map[string]string // field hints like {"State": "enum"}
}

// CollectionGenerateResult contains the result of smart collection schema generation.
type CollectionGenerateResult struct {
	CollectionSchema   *GenerateResult            // The collection schema (list type)
	NewItemSchemas     []*GenerateResult          // Any new item schemas that were generated
	ExistingSchemaRefs []string                   // References to existing schemas used
	ItemSchemaMapping  map[int]string             // Maps item index to schema name
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

// GenerateFromDetectedType generates a CUE schema from a detected type.
// If the detected type has an ImportPath (canonical schema available), it generates
// a schema that imports and extends the canonical type. Otherwise, it falls back
// to standard Generate() behavior.
func (g *Generator) GenerateFromDetectedType(
	detected *typepattern.DetectedType,
	sampleData interface{},
) (*GenerateResult, error) {
	if detected == nil {
		return nil, fmt.Errorf("detected type is nil")
	}

	if detected.Pattern == nil {
		return nil, fmt.Errorf("detected type has no pattern")
	}

	// Get ecosystem and definition name for file path
	ecosystem := detected.Pattern.Ecosystem
	definition := detected.Definition

	// If no import path, fall back to standard generation
	if detected.ImportPath == "" {
		opts := GenerateOptions{
			PackagePath:    fmt.Sprintf("pudl/%s", ecosystem),
			DefinitionName: definition,
			IsCollection:   false,
		}
		return g.Generate(sampleData, opts)
	}

	// Get PUDL metadata from pattern
	var metadata *typepattern.PudlMetadata
	if detected.Pattern.MetadataDefaults != nil {
		metadata = detected.Pattern.MetadataDefaults(detected.TypeID)
	}

	// Generate schema with import
	content := g.generateCUEContentWithImport(
		detected.ImportPath,
		definition,
		metadata,
		ecosystem,
	)

	// Determine file path: ~/.pudl/schema/pudl/<ecosystem>/<definition>.cue
	fileName := strings.ToLower(definition) + ".cue"
	filePath := filepath.Join(g.SchemaPath, "pudl", ecosystem, fileName)

	return &GenerateResult{
		FilePath:       filePath,
		PackageName:    ecosystem,
		DefinitionName: definition,
		FieldCount:     0, // Import-based schemas don't track field count
		Content:        content,
	}, nil
}

// generateCUEContentWithImport generates CUE content that imports and extends a canonical type.
// It creates proper CUE syntax with import statement and PUDL metadata block.
func (g *Generator) generateCUEContentWithImport(
	importPath string,
	definition string,
	metadata *typepattern.PudlMetadata,
	ecosystem string,
) string {
	var b strings.Builder

	// Package declaration
	b.WriteString(fmt.Sprintf("package %s\n\n", ecosystem))

	// Import statement with alias
	importAlias := g.deriveImportAlias(importPath)
	b.WriteString(fmt.Sprintf("import %s \"%s\"\n\n", importAlias, importPath))

	// Definition that extends the imported type
	b.WriteString(fmt.Sprintf("#%s: %s.#%s & {\n", definition, importAlias, definition))

	// Add _pudl metadata block
	b.WriteString("\t_pudl: {\n")

	if metadata != nil {
		b.WriteString(fmt.Sprintf("\t\tschema_type:      \"%s\"\n", metadata.SchemaType))
		b.WriteString(fmt.Sprintf("\t\tresource_type:    \"%s\"\n", metadata.ResourceType))
		b.WriteString(fmt.Sprintf("\t\tcascade_priority: %d\n", metadata.CascadePriority))
		b.WriteString(fmt.Sprintf("\t\tidentity_fields:  %s\n", g.formatStringSlice(metadata.IdentityFields)))
		b.WriteString(fmt.Sprintf("\t\ttracked_fields:   %s\n", g.formatStringSlice(metadata.TrackedFields)))
	} else {
		// Default metadata if none provided
		b.WriteString(fmt.Sprintf("\t\tschema_type:      \"base\"\n"))
		b.WriteString(fmt.Sprintf("\t\tresource_type:    \"%s.%s\"\n", ecosystem, strings.ToLower(definition)))
		b.WriteString("\t\tcascade_priority: 100\n")
		b.WriteString("\t\tidentity_fields:  []\n")
		b.WriteString("\t\ttracked_fields:   []\n")
	}

	b.WriteString("\t}\n")
	b.WriteString("}\n")

	return b.String()
}

// deriveImportAlias extracts an appropriate alias from an import path.
// For example, "cue.dev/x/k8s.io/api/batch/v1" returns "batch".
func (g *Generator) deriveImportAlias(importPath string) string {
	// Split by "/" and get the path segments
	parts := strings.Split(importPath, "/")
	if len(parts) == 0 {
		return "pkg"
	}

	// Get the last meaningful segment (skip version suffixes like "v1")
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		// Skip version-like suffixes
		if strings.HasPrefix(part, "v") && len(part) <= 3 {
			continue
		}
		// Skip common path components
		if part == "api" || part == "apis" {
			continue
		}
		// Use this as the alias
		return sanitizeIdentifier(part)
	}

	// Fallback to last segment
	return sanitizeIdentifier(parts[len(parts)-1])
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

	// Write _pudl metadata with inline documentation comments
	b.WriteString("\t_pudl: {\n")
	b.WriteString(fmt.Sprintf("\t\tschema_type: \"%s\" // Valid: \"base\", \"collection\", \"policy\", \"catchall\"\n", g.schemaType(opts)))
	b.WriteString(fmt.Sprintf("\t\tresource_type: \"%s.%s\" // Format: <package>.<type> - identifies this resource type\n", packageName, strings.ToLower(opts.DefinitionName)))
	b.WriteString("\t\tcascade_priority: 100 // 0-1000, higher = more specific (catchall=0, base=100, policy=200+)\n")
	b.WriteString("\t\tcascade_fallback: [\"pudl/core.#Item\"] // Schemas to try if this doesn't match\n")
	b.WriteString(fmt.Sprintf("\t\tidentity_fields: %s\n", g.formatStringSlice(analysis.IdentityFields)))
	b.WriteString(fmt.Sprintf("\t\ttracked_fields: %s\n", g.formatTrackedFields(analysis)))
	b.WriteString("\t\tcompliance_level: \"strict\" // Valid: \"strict\", \"warn\", \"permissive\"\n")
	b.WriteString("\t}\n\n")

	// Write field definitions
	g.writeFields(&b, analysis.Fields, 1)

	b.WriteString("}\n")

	return b.String()
}

// needsQuoting returns true if a CUE field name requires quoting.
// CUE identifiers must start with a letter or underscore and contain only
// letters, digits, underscores, and $. Any other characters require quoting.
func needsQuoting(name string) bool {
	if len(name) == 0 {
		return true
	}
	// First character must be letter, underscore, or $
	first := rune(name[0])
	if !unicode.IsLetter(first) && first != '_' && first != '$' {
		return true
	}
	// Remaining characters must be letter, digit, underscore, or $
	for _, r := range name[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
			return true
		}
	}
	return false
}

// formatFieldName returns the field name, quoted if necessary for valid CUE syntax.
func formatFieldName(name string) string {
	if needsQuoting(name) {
		return fmt.Sprintf("\"%s\"", name)
	}
	return name
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

		// Format field name with quoting if needed
		fieldName := formatFieldName(name)

		switch {
		case fi.CUEType == "enum" && len(fi.EnumValues) > 0:
			sort.Strings(fi.EnumValues)
			quoted := make([]string, len(fi.EnumValues))
			for i, v := range fi.EnumValues {
				quoted[i] = fmt.Sprintf("\"%s\"", v)
			}
			b.WriteString(fmt.Sprintf("%s%s%s: %s\n", indentStr, fieldName, optional, strings.Join(quoted, " | ")))

		case fi.CUEType == "struct" && fi.Nested != nil:
			b.WriteString(fmt.Sprintf("%s%s%s: {\n", indentStr, fieldName, optional))
			g.writeFields(b, fi.Nested, indent+1)
			b.WriteString(fmt.Sprintf("%s}\n", indentStr))

		case fi.CUEType == "object_array" && fi.Nested != nil:
			b.WriteString(fmt.Sprintf("%s%s%s: [...{\n", indentStr, fieldName, optional))
			g.writeFields(b, fi.Nested, indent+1)
			b.WriteString(fmt.Sprintf("%s}]\n", indentStr))

		default:
			typeStr := fi.CUEType
			if fi.IsNullable && typeStr != "_" {
				typeStr = fmt.Sprintf("null | %s", typeStr)
			}
			b.WriteString(fmt.Sprintf("%s%s%s: %s\n", indentStr, fieldName, optional, typeStr))
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

// SchemaExistsError is returned when a schema file already exists and force is not set.
type SchemaExistsError struct {
	FilePath       string
	DefinitionName string
	PackagePath    string
}

func (e *SchemaExistsError) Error() string {
	return fmt.Sprintf("schema file already exists: %s", e.FilePath)
}

// SchemaValidationError is returned when generated CUE content is invalid.
type SchemaValidationError struct {
	Content string
	Errors  []string
}

func (e *SchemaValidationError) Error() string {
	return fmt.Sprintf("generated schema has invalid CUE syntax: %s", strings.Join(e.Errors, "; "))
}

// ValidateCUEContent validates that the given content is valid CUE syntax.
// Returns nil if valid, or a SchemaValidationError with details if invalid.
func ValidateCUEContent(content string) error {
	// Parse the CUE content
	file, err := parser.ParseFile("generated.cue", content, parser.ParseComments)
	if err != nil {
		return &SchemaValidationError{
			Content: content,
			Errors:  []string{fmt.Sprintf("CUE syntax error: %v", err)},
		}
	}

	// Build CUE value to check for semantic errors
	ctx := cuecontext.New()
	value := ctx.BuildFile(file)
	if err := value.Err(); err != nil {
		return &SchemaValidationError{
			Content: content,
			Errors:  []string{fmt.Sprintf("CUE build error: %v", err)},
		}
	}

	// Validate the CUE value
	if err := value.Validate(); err != nil {
		return &SchemaValidationError{
			Content: content,
			Errors:  []string{fmt.Sprintf("CUE validation error: %v", err)},
		}
	}

	return nil
}

// ValidateCUESyntax validates only the syntax of CUE content without resolving imports.
// This is useful for import-based schemas where dependencies aren't yet available.
// Returns nil if syntax is valid, or a SchemaValidationError with details if invalid.
func ValidateCUESyntax(content string) error {
	// Parse the CUE content - this validates syntax without resolving imports
	_, err := parser.ParseFile("generated.cue", content, parser.ParseComments)
	if err != nil {
		return &SchemaValidationError{
			Content: content,
			Errors:  []string{fmt.Sprintf("CUE syntax error: %v", err)},
		}
	}
	return nil
}

// WriteSchema writes the generated schema to the schema repository.
// It validates the CUE content before writing to prevent invalid schemas.
// If force is true, existing files will be overwritten.
func (g *Generator) WriteSchema(result *GenerateResult, content string, force bool) error {
	// Validate CUE content before writing
	if err := ValidateCUEContent(content); err != nil {
		return err
	}

	return g.writeSchemaFile(result, content, force)
}

// WriteSchemaWithSyntaxCheck writes the generated schema with syntax-only validation.
// This is useful for import-based schemas where dependencies aren't available until
// after the file is written and `cue mod tidy` is run.
// If force is true, existing files will be overwritten.
func (g *Generator) WriteSchemaWithSyntaxCheck(result *GenerateResult, content string, force bool) error {
	// Validate CUE syntax only (doesn't try to resolve imports)
	if err := ValidateCUESyntax(content); err != nil {
		return err
	}

	return g.writeSchemaFile(result, content, force)
}

// writeSchemaFile is the internal implementation for writing schema files.
func (g *Generator) writeSchemaFile(result *GenerateResult, content string, force bool) error {
	// Create package directory
	dir := filepath.Dir(result.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create package directory: %w", err)
	}

	// Check if file already exists
	if _, err := os.Stat(result.FilePath); err == nil {
		if !force {
			return &SchemaExistsError{
				FilePath:       result.FilePath,
				DefinitionName: result.DefinitionName,
				PackagePath:    result.PackageName,
			}
		}
		// force=true, file will be overwritten
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

// GenerateSmartCollection generates a collection schema by inferring item schemas.
// It runs inference on each item, groups by matched schema, generates new schemas
// for unmatched items, and creates a collection schema as a union of all item types.
func (g *Generator) GenerateSmartCollection(items []interface{}, opts CollectionGenerateOptions, inferrer *inference.SchemaInferrer) (*CollectionGenerateResult, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("collection has no items")
	}

	result := &CollectionGenerateResult{
		NewItemSchemas:     []*GenerateResult{},
		ExistingSchemaRefs: []string{},
		ItemSchemaMapping:  make(map[int]string),
	}

	// Track which schemas are used and which items need new schemas
	schemaUsage := make(map[string][]int)       // schema name -> item indices
	unmatchedItems := make(map[int]interface{}) // item index -> item data

	// Run inference on each item
	for i, item := range items {
		hints := inference.InferenceHints{
			CollectionType: "item", // We're inferring item schemas, not collection schemas
		}

		inferResult, err := inferrer.Infer(item, hints)
		if err != nil {
			return nil, fmt.Errorf("failed to infer schema for item %d: %w", i, err)
		}

		// Check if it matched a real schema or fell back to catchall
		if isCatchallSchema(inferResult.Schema) || inferResult.Confidence < 0.5 {
			// Item doesn't match any existing schema well enough
			unmatchedItems[i] = item
		} else {
			// Item matches an existing schema
			schemaUsage[inferResult.Schema] = append(schemaUsage[inferResult.Schema], i)
			result.ItemSchemaMapping[i] = inferResult.Schema
		}
	}

	// Collect existing schema references
	for schemaName := range schemaUsage {
		result.ExistingSchemaRefs = append(result.ExistingSchemaRefs, schemaName)
	}
	sort.Strings(result.ExistingSchemaRefs)

	// Generate new item schemas for unmatched items
	if len(unmatchedItems) > 0 {
		newSchema, err := g.generateItemSchemaForUnmatched(unmatchedItems, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to generate item schema: %w", err)
		}
		result.NewItemSchemas = append(result.NewItemSchemas, newSchema)

		// Map unmatched items to the new schema
		newSchemaRef := fmt.Sprintf("pudl.schemas/%s:#%s", opts.PackagePath, newSchema.DefinitionName)
		for idx := range unmatchedItems {
			result.ItemSchemaMapping[idx] = newSchemaRef
		}
	}

	// Generate the collection schema as a list type
	collectionResult, err := g.generateCollectionListSchema(opts, result.ExistingSchemaRefs, result.NewItemSchemas)
	if err != nil {
		return nil, fmt.Errorf("failed to generate collection schema: %w", err)
	}
	result.CollectionSchema = collectionResult

	return result, nil
}

// isCatchallSchema checks if a schema name is a catchall/fallback schema.
func isCatchallSchema(name string) bool {
	return strings.Contains(name, "CatchAll") || strings.Contains(name, "catchall") ||
		name == "core.#Item" || strings.HasSuffix(name, ":#Item")
}

// generateItemSchemaForUnmatched generates a new item schema from unmatched items.
func (g *Generator) generateItemSchemaForUnmatched(items map[int]interface{}, opts CollectionGenerateOptions) (*GenerateResult, error) {
	// Convert map to slice for analysis
	var itemSlice []interface{}
	for _, item := range items {
		itemSlice = append(itemSlice, item)
	}

	// Derive item schema name from collection name (remove "Collection" suffix if present)
	itemName := opts.CollectionName
	if strings.HasSuffix(itemName, "Collection") {
		itemName = strings.TrimSuffix(itemName, "Collection")
	} else {
		itemName = itemName + "Item"
	}

	genOpts := GenerateOptions{
		PackagePath:    opts.PackagePath,
		DefinitionName: itemName,
		IsCollection:   false,
		InferHints:     opts.InferHints,
	}

	return g.Generate(itemSlice, genOpts)
}

// generateCollectionListSchema generates a collection schema as a list type.
func (g *Generator) generateCollectionListSchema(opts CollectionGenerateOptions, existingRefs []string, newSchemas []*GenerateResult) (*GenerateResult, error) {
	packageName := filepath.Base(opts.PackagePath)
	fileName := strings.ToLower(opts.CollectionName) + ".cue"
	filePath := filepath.Join(g.SchemaPath, opts.PackagePath, fileName)

	// Parse schema references and collect imports needed
	type schemaRef struct {
		pkgPath    string // e.g., "aws/ec2"
		pkgAlias   string // e.g., "ec2" (last component)
		defName    string // e.g., "#Instance"
		isLocal    bool   // true if in same package
	}

	var refs []schemaRef
	importPaths := make(map[string]string) // pkgPath -> alias

	// Process existing schema references
	for _, ref := range existingRefs {
		parsed := parseSchemaRef(ref, opts.PackagePath)
		refs = append(refs, parsed)
		if !parsed.isLocal {
			importPaths[parsed.pkgPath] = parsed.pkgAlias
		}
	}

	// Add new schema references (always local)
	for _, schema := range newSchemas {
		refs = append(refs, schemaRef{
			defName: "#" + schema.DefinitionName,
			isLocal: true,
		})
	}

	// Generate the CUE content
	var b strings.Builder
	b.WriteString(fmt.Sprintf("package %s\n", packageName))

	// Add imports if needed
	if len(importPaths) > 0 {
		b.WriteString("\nimport (\n")
		// Sort for deterministic output
		var paths []string
		for path := range importPaths {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			alias := importPaths[path]
			// Use pudl.schemas module path
			b.WriteString(fmt.Sprintf("\t%s \"pudl.schemas/%s\"\n", alias, path))
		}
		b.WriteString(")\n")
	}
	b.WriteString("\n")

	// Build the union of all item types
	var itemTypes []string
	for _, ref := range refs {
		if ref.isLocal {
			itemTypes = append(itemTypes, ref.defName)
		} else {
			// Use alias.#Definition format
			itemTypes = append(itemTypes, ref.pkgAlias+"."+ref.defName)
		}
	}

	// Build the list type expression
	var listType string
	if len(itemTypes) == 1 {
		listType = fmt.Sprintf("[...%s]", itemTypes[0])
	} else {
		listType = fmt.Sprintf("[...(%s)]", strings.Join(itemTypes, " | "))
	}

	b.WriteString(fmt.Sprintf("#%s: %s\n", opts.CollectionName, listType))

	return &GenerateResult{
		FilePath:       filePath,
		PackageName:    packageName,
		DefinitionName: opts.CollectionName,
		FieldCount:     len(itemTypes),
		Content:        b.String(),
	}, nil
}

// parseSchemaRef parses a schema reference and determines if it's local or needs import.
func parseSchemaRef(ref string, currentPackage string) struct {
	pkgPath  string
	pkgAlias string
	defName  string
	isLocal  bool
} {
	result := struct {
		pkgPath  string
		pkgAlias string
		defName  string
		isLocal  bool
	}{}

	// Parse the reference: "pudl.schemas/aws/ec2:#Instance" or "pudl.schemas/aws/ec2@v0:#Instance"
	parts := strings.Split(ref, ":")
	if len(parts) != 2 {
		result.defName = ref
		result.isLocal = true
		return result
	}

	result.defName = parts[1] // e.g., "#Instance"
	pkgPath := parts[0]       // e.g., "pudl.schemas/aws/ec2" or "pudl.schemas/aws/ec2@v0"

	// Remove "pudl.schemas/" prefix and version suffix
	pkgPath = strings.TrimPrefix(pkgPath, "pudl.schemas/")
	if idx := strings.Index(pkgPath, "@"); idx != -1 {
		pkgPath = pkgPath[:idx]
	}

	result.pkgPath = pkgPath
	// Use last path component as alias
	pathParts := strings.Split(pkgPath, "/")
	result.pkgAlias = pathParts[len(pathParts)-1]

	// Check if same package
	result.isLocal = (pkgPath == currentPackage)

	return result
}