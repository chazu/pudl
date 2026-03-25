package definition

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Discoverer finds and parses definition files from the definitions directory.
type Discoverer struct {
	searchPaths []string // ordered list of schema paths to search
}

// NewDiscoverer creates a discoverer with a single schema path (backward compatible).
func NewDiscoverer(schemaPath string) *Discoverer {
	return &Discoverer{searchPaths: []string{schemaPath}}
}

// NewMultiDiscoverer creates a discoverer that searches multiple paths in order.
// Definitions found in earlier paths shadow definitions with the same name in later paths.
func NewMultiDiscoverer(schemaPaths []string) *Discoverer {
	return &Discoverer{searchPaths: schemaPaths}
}

// definitionsDirs returns the paths to all definitions directories.
func (d *Discoverer) definitionsDirs() []string {
	var dirs []string
	for _, sp := range d.searchPaths {
		dirs = append(dirs, filepath.Join(sp, "definitions"))
	}
	return dirs
}

// ListDefinitions finds all definitions across all search paths.
// Definitions in earlier paths shadow definitions with the same name in later paths.
func (d *Discoverer) ListDefinitions() ([]DefinitionInfo, error) {
	seen := make(map[string]bool)
	var definitions []DefinitionInfo

	for _, defsDir := range d.definitionsDirs() {
		if _, err := os.Stat(defsDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(defsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".cue") {
				return nil
			}

			fileDefs, err := d.parseDefinitionsFromFile(path)
			if err != nil {
				return nil // Skip files that can't be parsed
			}

			for _, def := range fileDefs {
				if !seen[def.Name] {
					seen[def.Name] = true
					definitions = append(definitions, def)
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk definitions directory %s: %w", defsDir, err)
		}
	}

	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})

	return definitions, nil
}

// GetDefinition finds a specific definition by name.
func (d *Discoverer) GetDefinition(name string) (*DefinitionInfo, error) {
	definitions, err := d.ListDefinitions()
	if err != nil {
		return nil, err
	}

	for _, def := range definitions {
		if def.Name == name {
			return &def, nil
		}
	}

	return nil, fmt.Errorf("definition not found: %s", name)
}

// Regex patterns for definition detection
var (
	// Matches: name: pkg.#SomeSchema & {
	schemaUnifyPattern = regexp.MustCompile(`^(\w+)\s*:\s*(\S+\.#\w+)\s*&`)
	// Matches: name: { with _schema marker inside
	markerPattern = regexp.MustCompile(`_schema:\s*"([^"]+)"`)
	// Matches cross-definition references like: prod_vpc.outputs.vpc_id
	crossRefPattern = regexp.MustCompile(`(\w+)\.(outputs|schema)\.\w+`)
)

// parseDefinitionsFromFile extracts definition declarations from a CUE file.
// Detection heuristic: a CUE value that unifies against a #Schema type
// (text contains pkg.#Name &) is a definition.
func (d *Discoverer) parseDefinitionsFromFile(filePath string) ([]DefinitionInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	text := string(content)
	lines := strings.Split(text, "\n")

	// Extract package name
	packageName := "definitions"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			packageName = strings.TrimPrefix(trimmed, "package ")
			break
		}
	}

	var definitions []DefinitionInfo

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "import") || strings.HasPrefix(trimmed, "package") {
			continue
		}

		// Check for schema unification pattern: name: pkg.#Schema & {
		if matches := schemaUnifyPattern.FindStringSubmatch(trimmed); len(matches) > 2 {
			defName := matches[1]
			schemaRef := matches[2]

			body := extractBody(lines, i)
			bindings := extractSocketBindings(body)

			definitions = append(definitions, DefinitionInfo{
				Name:           defName,
				SchemaRef:      schemaRef,
				Package:        packageName,
				FilePath:       filePath,
				SocketBindings: bindings,
			})
			continue
		}

		// Check for marker-based definitions: name: { ... _schema: "..." ... }
		if matches := regexp.MustCompile(`^(\w+)\s*:\s*\{`).FindStringSubmatch(trimmed); len(matches) > 1 {
			defName := matches[1]
			body := extractBody(lines, i)

			if markerMatches := markerPattern.FindStringSubmatch(body); len(markerMatches) > 1 {
				definitions = append(definitions, DefinitionInfo{
					Name:           defName,
					SchemaRef:      markerMatches[1],
					Package:        packageName,
					FilePath:       filePath,
					SocketBindings: extractSocketBindings(body),
				})
			}
		}
	}

	return definitions, nil
}

// extractBody extracts the body text of a definition starting at the given line.
func extractBody(lines []string, startLine int) string {
	depth := 0
	inBody := false
	var bodyLines []string

	for i := startLine; i < len(lines); i++ {
		bodyLines = append(bodyLines, lines[i])
		for _, ch := range lines[i] {
			switch ch {
			case '{':
				depth++
				inBody = true
			case '}':
				depth--
				if inBody && depth == 0 {
					return strings.Join(bodyLines, "\n")
				}
			}
		}
	}

	return strings.Join(bodyLines, "\n")
}

// extractSocketBindings finds cross-definition references in a definition body.
// These represent socket wiring — where one definition references another's outputs.
func extractSocketBindings(body string) map[string]string {
	bindings := make(map[string]string)

	// Find all cross-definition references
	matches := crossRefPattern.FindAllStringSubmatch(body, -1)
	for _, match := range matches {
		if len(match) > 0 {
			// Find the field being assigned: look for "fieldName: ref.outputs.field"
			assignPattern := regexp.MustCompile(`(\w+):\s*` + regexp.QuoteMeta(match[0]))
			lines := strings.Split(body, "\n")
			for _, line := range lines {
				if assigns := assignPattern.FindStringSubmatch(line); len(assigns) > 1 {
					bindings[assigns[1]] = match[0]
				}
			}
		}
	}

	return bindings
}
