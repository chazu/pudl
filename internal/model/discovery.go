package model

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Discoverer finds and parses model definitions from CUE files.
type Discoverer struct {
	schemaPath string
}

// NewDiscoverer creates a new model discoverer.
func NewDiscoverer(schemaPath string) *Discoverer {
	return &Discoverer{schemaPath: schemaPath}
}

// ListModels finds all models in the schema path.
func (d *Discoverer) ListModels() ([]ModelInfo, error) {
	var models []ModelInfo

	err := filepath.Walk(d.schemaPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip cue.mod directory
		if info.IsDir() && info.Name() == "cue.mod" {
			return filepath.SkipDir
		}

		// Only process .cue files
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".cue") {
			return nil
		}

		fileModels, err := d.parseModelsFromFile(path)
		if err != nil {
			return nil // Skip files that can't be parsed
		}

		models = append(models, fileModels...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk schema directory: %w", err)
	}

	// Also walk extensions/models/ for user-defined models
	extModelsDir := filepath.Join(d.schemaPath, "extensions", "models")
	if info, err := os.Stat(extModelsDir); err == nil && info.IsDir() {
		_ = filepath.Walk(extModelsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && info.Name() == "cue.mod" {
				return filepath.SkipDir
			}
			if info.IsDir() || !strings.HasSuffix(info.Name(), ".cue") {
				return nil
			}
			fileModels, err := d.parseModelsFromFile(path)
			if err != nil {
				return nil
			}
			models = append(models, fileModels...)
			return nil
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models, nil
}

// GetModel finds a specific model by name.
func (d *Discoverer) GetModel(name string) (*ModelInfo, error) {
	models, err := d.ListModels()
	if err != nil {
		return nil, err
	}

	for _, m := range models {
		if m.Name == name {
			return &m, nil
		}
	}

	return nil, fmt.Errorf("model not found: %s", name)
}

// parseModelsFromFile extracts model definitions from a CUE file using
// text-based parsing. A definition is considered a model if it has both
// a "metadata:" field and a "methods:" field within its body.
func (d *Discoverer) parseModelsFromFile(filePath string) ([]ModelInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(d.schemaPath, filePath)
	if err != nil {
		relPath = filePath
	}
	packageName := filepath.Dir(relPath)
	if packageName == "." {
		packageName = "root"
	}

	text := string(content)
	var models []ModelInfo

	// Find all top-level definitions that reference #Model
	defPattern := regexp.MustCompile(`(?m)^(#[A-Z][a-zA-Z0-9]*)\s*:\s*.*#Model`)
	matches := defPattern.FindAllStringSubmatchIndex(text, -1)

	for _, match := range matches {
		defName := text[match[2]:match[3]]
		defStart := match[0]

		// Extract the body of this definition (from the match to the next
		// top-level definition or end of file)
		body := extractDefinitionBody(text, defStart)

		modelInfo := ModelInfo{
			Name:     fmt.Sprintf("%s.%s", packageName, defName),
			Package:  packageName,
			FilePath: filePath,
			Methods:  make(map[string]Method),
			Sockets:  make(map[string]Socket),
		}

		parseModelBody(body, &modelInfo)
		models = append(models, modelInfo)
	}

	return models, nil
}

// extractDefinitionBody returns the text from the definition start to the
// next top-level definition or end of file, properly handling brace nesting.
func extractDefinitionBody(text string, start int) string {
	depth := 0
	inBody := false
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
			inBody = true
		case '}':
			depth--
			if inBody && depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return text[start:]
}

var (
	metadataNameRe     = regexp.MustCompile(`name:\s*"([^"]+)"`)
	metadataDescRe     = regexp.MustCompile(`description:\s*"([^"]+)"`)
	metadataCategoryRe = regexp.MustCompile(`category:\s*"([^"]+)"`)
	metadataIconRe     = regexp.MustCompile(`icon:\s*"([^"]+)"`)
	methodBlockRe      = regexp.MustCompile(`(?m)^\s+(\w+):\s*.*#Method`)
	kindRe             = regexp.MustCompile(`kind:\s*"([^"]+)"`)
	descriptionRe      = regexp.MustCompile(`description:\s*"([^"]+)"`)
	timeoutRe          = regexp.MustCompile(`timeout:\s*"([^"]+)"`)
	retriesRe          = regexp.MustCompile(`retries:\s*(\d+)`)
	blocksRe           = regexp.MustCompile(`blocks:\s*\[([^\]]*)\]`)
	socketBlockRe      = regexp.MustCompile(`(?m)^\s+(\w+):\s*.*#Socket`)
	directionRe        = regexp.MustCompile(`direction:\s*"([^"]+)"`)
	requiredRe         = regexp.MustCompile(`required:\s*(true|false)`)
	authMethodRe       = regexp.MustCompile(`method:\s*"(bearer|sigv4|basic|custom)"`)
)

// parseModelBody extracts metadata, methods, sockets, and auth from a model body.
func parseModelBody(body string, info *ModelInfo) {
	// Parse metadata section
	metaStart := strings.Index(body, "metadata:")
	if metaStart >= 0 {
		metaBody := extractSection(body, metaStart)
		if m := metadataNameRe.FindStringSubmatch(metaBody); len(m) > 1 {
			info.Metadata.Name = m[1]
		}
		if m := metadataDescRe.FindStringSubmatch(metaBody); len(m) > 1 {
			info.Metadata.Description = m[1]
		}
		if m := metadataCategoryRe.FindStringSubmatch(metaBody); len(m) > 1 {
			info.Metadata.Category = m[1]
		}
		if m := metadataIconRe.FindStringSubmatch(metaBody); len(m) > 1 {
			info.Metadata.Icon = m[1]
		}
	}

	// Parse methods section
	methodsStart := strings.Index(body, "methods:")
	if methodsStart >= 0 {
		methodsBody := extractSection(body, methodsStart)
		methodMatches := methodBlockRe.FindAllStringSubmatchIndex(methodsBody, -1)
		for _, match := range methodMatches {
			methodName := methodsBody[match[2]:match[3]]
			methodBody := extractMethodBody(methodsBody, match[0])
			method := parseMethod(methodBody)
			info.Methods[methodName] = method
		}
	}

	// Parse sockets section
	socketsStart := strings.Index(body, "sockets:")
	if socketsStart >= 0 {
		socketsBody := extractSection(body, socketsStart)
		socketMatches := socketBlockRe.FindAllStringSubmatchIndex(socketsBody, -1)
		for _, match := range socketMatches {
			socketName := socketsBody[match[2]:match[3]]
			socketBody := extractMethodBody(socketsBody, match[0])
			socket := parseSocket(socketBody)
			info.Sockets[socketName] = socket
		}
	}

	// Parse auth section
	authStart := strings.Index(body, "auth:")
	if authStart >= 0 {
		authBody := extractSection(body, authStart)
		if m := authMethodRe.FindStringSubmatch(authBody); len(m) > 1 {
			info.Auth = &AuthConfig{Method: m[1]}
		}
	}
}

// extractSection returns the text for a section starting at the given offset,
// from the section keyword to the closing brace of its block.
func extractSection(body string, start int) string {
	depth := 0
	inSection := false
	for i := start; i < len(body); i++ {
		switch body[i] {
		case '{':
			depth++
			inSection = true
		case '}':
			depth--
			if inSection && depth == 0 {
				return body[start : i+1]
			}
		}
	}
	return body[start:]
}

// extractMethodBody returns the text for a single method/socket entry.
func extractMethodBody(body string, start int) string {
	depth := 0
	inBody := false
	for i := start; i < len(body); i++ {
		switch body[i] {
		case '{':
			depth++
			inBody = true
		case '}':
			depth--
			if inBody && depth == 0 {
				return body[start : i+1]
			}
		}
	}
	return body[start:]
}

func parseMethod(body string) Method {
	m := Method{
		Kind:    "action", // default
		Timeout: "5m",     // default
	}

	if match := kindRe.FindStringSubmatch(body); len(match) > 1 {
		m.Kind = match[1]
	}
	if match := descriptionRe.FindStringSubmatch(body); len(match) > 1 {
		m.Description = match[1]
	}
	if match := timeoutRe.FindStringSubmatch(body); len(match) > 1 {
		m.Timeout = match[1]
	}
	if match := retriesRe.FindStringSubmatch(body); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &m.Retries)
	}
	if match := blocksRe.FindStringSubmatch(body); len(match) > 1 {
		m.Blocks = parseStringList(match[1])
	}

	return m
}

func parseSocket(body string) Socket {
	s := Socket{
		Required: true, // default
	}

	if match := directionRe.FindStringSubmatch(body); len(match) > 1 {
		s.Direction = match[1]
	}
	if match := descriptionRe.FindStringSubmatch(body); len(match) > 1 {
		s.Description = match[1]
	}
	if match := requiredRe.FindStringSubmatch(body); len(match) > 1 {
		s.Required = match[1] == "true"
	}

	return s
}

// parseStringList parses a CUE list of quoted strings like `"a", "b"`.
func parseStringList(s string) []string {
	var result []string
	re := regexp.MustCompile(`"([^"]+)"`)
	matches := re.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		result = append(result, m[1])
	}
	return result
}
