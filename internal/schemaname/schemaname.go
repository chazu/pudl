// Package schemaname provides utilities for parsing, normalizing, and formatting
// schema names in the PUDL system.
//
// Canonical format: <package-path>.#<DefinitionName>
// Examples: aws/ec2.#Instance, pudl/core.#Item, k8s/v1.#Pod
//
// CUE internal format: pudl.schemas/<package-path>@v0:#<DefinitionName>
// Examples: pudl.schemas/aws/ec2@v0:#Instance
package schemaname

import (
	"regexp"
	"strings"
)

// versionSuffixRegex matches @v0, @v1, etc.
var versionSuffixRegex = regexp.MustCompile(`@v\d+`)

// Normalize converts any schema name format to canonical format.
//
// Input examples:
//
//	"pudl.schemas/aws/ec2@v0:#Instance" → "aws/ec2.#Instance"
//	"pudl.schemas/aws/ec2:#Instance"    → "aws/ec2.#Instance"
//	"aws/ec2:#Instance"                 → "aws/ec2.#Instance"
//	"aws/ec2.#Instance"                 → "aws/ec2.#Instance"
//	"core.#Item"                        → "pudl/core.#Item"
func Normalize(name string) string {
	if name == "" {
		return ""
	}

	// Step 1: Strip "pudl.schemas/" prefix if present
	name = strings.TrimPrefix(name, "pudl.schemas/")

	// Step 2: Strip version suffix (@v0, @v1, etc.)
	name = versionSuffixRegex.ReplaceAllString(name, "")

	// Step 3: Convert ":" separator to "."
	name = strings.Replace(name, ":#", ".#", 1)
	name = strings.Replace(name, ":", ".#", 1) // Handle case without # after :

	// Step 4: Handle legacy short names (core.#Item -> pudl/core.#Item)
	if strings.HasPrefix(name, "core.#") {
		name = "pudl/" + name
	}

	// Step 5: Ensure # prefix on definition if missing
	if idx := strings.LastIndex(name, "."); idx != -1 {
		pkg := name[:idx]
		def := name[idx+1:]
		if !strings.HasPrefix(def, "#") && def != "" {
			name = pkg + ".#" + def
		}
	}

	return name
}

// Parse extracts package path and definition from any schema name format.
// Returns the package path (e.g., "aws/ec2") and definition (e.g., "#Instance").
func Parse(name string) (pkgPath string, definition string, err error) {
	normalized := Normalize(name)
	if normalized == "" {
		return "", "", nil
	}

	// Find the last "." that precedes a "#"
	idx := strings.LastIndex(normalized, ".#")
	if idx == -1 {
		// Try finding just "." as separator
		idx = strings.LastIndex(normalized, ".")
		if idx == -1 {
			// No separator found, treat entire string as package
			return normalized, "", nil
		}
	}

	pkgPath = normalized[:idx]
	definition = normalized[idx+1:]

	// Ensure definition has # prefix
	if !strings.HasPrefix(definition, "#") {
		definition = "#" + definition
	}

	return pkgPath, definition, nil
}

// Format creates canonical format from package path and definition components.
// The definition can be provided with or without the # prefix.
func Format(pkgPath, definition string) string {
	if pkgPath == "" {
		return ""
	}

	// Strip version suffix from package path
	pkgPath = versionSuffixRegex.ReplaceAllString(pkgPath, "")

	// Ensure definition has # prefix
	if definition != "" && !strings.HasPrefix(definition, "#") {
		definition = "#" + definition
	}

	if definition == "" {
		return pkgPath
	}

	return pkgPath + "." + definition
}

// IsEquivalent checks if two schema names refer to the same schema.
// Both names are normalized before comparison.
func IsEquivalent(a, b string) bool {
	return Normalize(a) == Normalize(b)
}

// StripDefinition returns just the package path from a schema name.
func StripDefinition(name string) string {
	pkgPath, _, _ := Parse(name)
	return pkgPath
}

// GetDefinition returns just the definition from a schema name.
func GetDefinition(name string) string {
	_, def, _ := Parse(name)
	return def
}

// IsFallbackSchema checks if the schema name refers to a fallback/catchall schema.
func IsFallbackSchema(name string) bool {
	normalized := Normalize(name)
	return normalized == "pudl/core.#Item" ||
		normalized == "pudl/core.#Collection" ||
		normalized == "pudl/core.#CatchAll"
}

