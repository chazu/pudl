package streaming

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// GenericChunkProcessor handles chunks of unknown format
type GenericChunkProcessor struct{}

// NewGenericChunkProcessor creates a new generic chunk processor
func NewGenericChunkProcessor() *GenericChunkProcessor {
	return &GenericChunkProcessor{}
}

// ProcessChunk processes a chunk with unknown format
func (p *GenericChunkProcessor) ProcessChunk(chunk *CDCChunk) (*ProcessedChunk, error) {
	// Try to detect the format
	format := p.detectFormat(chunk.Data)

	// Create basic processed chunk
	processed := &ProcessedChunk{
		Original:   chunk,
		Format:     format,
		Objects:    []interface{}{},
		Metadata:   make(map[string]interface{}),
		Errors:     []error{},
		Partial:    false,
		Boundaries: []int{},
	}

	// Add basic metadata
	processed.Metadata["size"] = chunk.Size
	processed.Metadata["format"] = format
	processed.Metadata["detected_at"] = chunk.Time

	// Try to extract some basic information based on format
	switch format {
	case "json":
		return p.processJSONChunk(chunk, processed)
	case "yaml":
		return p.processYAMLChunk(chunk, processed)
	case "text":
		return p.processTextChunk(chunk, processed)
	case "binary":
		return p.processBinaryChunk(chunk, processed)
	default:
		// Unknown format, treat as raw data
		processed.Objects = append(processed.Objects, map[string]interface{}{
			"raw_data": chunk.Data,
			"size":     chunk.Size,
			"format":   "unknown",
		})
	}

	return processed, nil
}

// CanProcess returns true if this processor can handle the given data
func (p *GenericChunkProcessor) CanProcess(data []byte) bool {
	// Generic processor can handle any data
	return true
}

// FormatName returns the name of the format this processor handles
func (p *GenericChunkProcessor) FormatName() string {
	return "generic"
}

// detectFormat attempts to detect the format of the data
func (p *GenericChunkProcessor) detectFormat(data []byte) string {
	if len(data) == 0 {
		return "empty"
	}

	// Trim whitespace for analysis
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "whitespace"
	}

	// Check for JSON
	if p.looksLikeJSON(trimmed) {
		return "json"
	}

	// Check for CSV
	if p.looksLikeCSV(trimmed) {
		return "csv"
	}

	// Check for YAML
	if p.looksLikeYAML(trimmed) {
		return "yaml"
	}

	// Check if it's mostly text
	if p.isText(data) {
		return "text"
	}

	return "binary"
}

// looksLikeJSON checks if data looks like JSON
func (p *GenericChunkProcessor) looksLikeJSON(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	// Check for JSON object or array start
	first := data[0]
	if first == '{' || first == '[' {
		// Try to parse as JSON
		var obj interface{}
		return json.Unmarshal(data, &obj) == nil
	}

	return false
}

// looksLikeCSV checks if data looks like CSV
func (p *GenericChunkProcessor) looksLikeCSV(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 2 {
		return false
	}

	// Check if first few lines have consistent comma count
	firstLineCommas := bytes.Count(lines[0], []byte(","))
	if firstLineCommas == 0 {
		return false
	}

	consistentLines := 0
	for i := 1; i < len(lines) && i < 5; i++ {
		if len(lines[i]) == 0 {
			continue
		}
		if bytes.Count(lines[i], []byte(",")) == firstLineCommas {
			consistentLines++
		}
	}

	return consistentLines >= 2
}

// looksLikeYAML checks if data looks like YAML
func (p *GenericChunkProcessor) looksLikeYAML(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	lines := bytes.Split(data, []byte("\n"))
	yamlIndicators := 0

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		// Look for YAML indicators
		if bytes.Contains(line, []byte(": ")) ||
			bytes.HasPrefix(line, []byte("- ")) ||
			bytes.HasPrefix(line, []byte("---")) {
			yamlIndicators++
		}
	}

	return yamlIndicators > 0
}

// isText checks if data is mostly text
func (p *GenericChunkProcessor) isText(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	textChars := 0
	for _, b := range data {
		if unicode.IsPrint(rune(b)) || unicode.IsSpace(rune(b)) {
			textChars++
		}
	}

	// Consider it text if more than 80% of characters are printable
	return float64(textChars)/float64(len(data)) > 0.8
}

// processJSONChunk processes a chunk that looks like JSON
func (p *GenericChunkProcessor) processJSONChunk(chunk *CDCChunk, processed *ProcessedChunk) (*ProcessedChunk, error) {
	// Try to parse as JSON
	var obj interface{}
	if err := json.Unmarshal(chunk.Data, &obj); err != nil {
		// Not valid JSON, treat as text
		processed.Format = "text"
		processed.Errors = append(processed.Errors, fmt.Errorf("invalid JSON: %w", err))
		return p.processTextChunk(chunk, processed)
	}

	// Handle arrays by extracting individual elements (matches legacy behavior)
	if arr, ok := obj.([]interface{}); ok {
		processed.Objects = append(processed.Objects, arr...)
		processed.Metadata["json_type"] = "array"
		processed.Metadata["array_length"] = len(arr)
	} else {
		processed.Objects = append(processed.Objects, obj)
		processed.Metadata["json_type"] = getJSONType(obj)
	}

	return processed, nil
}

// processYAMLChunk processes a chunk that looks like YAML
func (p *GenericChunkProcessor) processYAMLChunk(chunk *CDCChunk, processed *ProcessedChunk) (*ProcessedChunk, error) {
	// Try to parse as YAML using the standard library
	var obj interface{}
	if err := yaml.Unmarshal(chunk.Data, &obj); err != nil {
		// Not valid YAML, treat as text
		processed.Format = "text"
		processed.Errors = append(processed.Errors, fmt.Errorf("invalid YAML: %w", err))
		return p.processTextChunk(chunk, processed)
	}

	processed.Objects = append(processed.Objects, obj)
	processed.Metadata["yaml_type"] = getJSONType(obj) // YAML and JSON have similar types

	return processed, nil
}

// processTextChunk processes a chunk that looks like text
func (p *GenericChunkProcessor) processTextChunk(chunk *CDCChunk, processed *ProcessedChunk) (*ProcessedChunk, error) {
	text := string(chunk.Data)
	lines := strings.Split(text, "\n")

	// Create a text object
	textObj := map[string]interface{}{
		"content":    text,
		"line_count": len(lines),
		"char_count": len(text),
		"byte_count": len(chunk.Data),
	}

	// Add some basic text analysis
	if len(lines) > 0 {
		textObj["first_line"] = lines[0]
		if len(lines) > 1 {
			textObj["last_line"] = lines[len(lines)-1]
		}
	}

	processed.Objects = append(processed.Objects, textObj)
	processed.Metadata["encoding"] = "utf-8" // Assume UTF-8 for now

	return processed, nil
}

// processBinaryChunk processes a chunk that looks like binary data
func (p *GenericChunkProcessor) processBinaryChunk(chunk *CDCChunk, processed *ProcessedChunk) (*ProcessedChunk, error) {
	// Create a binary object with basic information
	binaryObj := map[string]interface{}{
		"size":        chunk.Size,
		"type":        "binary",
		"hash":        chunk.Hash,
		"first_bytes": getFirstBytes(chunk.Data, 16),
	}

	processed.Objects = append(processed.Objects, binaryObj)
	processed.Metadata["encoding"] = "binary"

	return processed, nil
}

// Helper functions

// getJSONType returns the type of a JSON object
func getJSONType(obj interface{}) string {
	switch obj.(type) {
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

// getFirstBytes returns the first n bytes of data as a hex string
func getFirstBytes(data []byte, n int) string {
	if len(data) < n {
		n = len(data)
	}
	if n == 0 {
		return ""
	}

	return fmt.Sprintf("%x", data[:n])
}

// ProcessorRegistry manages chunk processors
type ProcessorRegistry struct {
	processors map[string]ChunkProcessor
}

// NewProcessorRegistry creates a new processor registry
func NewProcessorRegistry() *ProcessorRegistry {
	registry := &ProcessorRegistry{
		processors: make(map[string]ChunkProcessor),
	}

	// Register format-specific processors
	registry.Register(NewJSONChunkProcessor())
	registry.Register(NewCSVChunkProcessor())
	registry.Register(NewYAMLChunkProcessor())

	// Register the generic processor as fallback
	registry.Register(NewGenericChunkProcessor())

	return registry
}

// Register adds a processor to the registry
func (r *ProcessorRegistry) Register(processor ChunkProcessor) {
	r.processors[processor.FormatName()] = processor
}

// GetProcessor returns a processor for the given format
func (r *ProcessorRegistry) GetProcessor(format string) ChunkProcessor {
	if processor, exists := r.processors[format]; exists {
		return processor
	}
	return r.processors["generic"] // Fallback to generic
}

// GetBestProcessor returns the best processor for the given data
func (r *ProcessorRegistry) GetBestProcessor(data []byte) ChunkProcessor {
	// Try specialized processors in priority order
	processorOrder := []string{"json", "csv", "yaml"}

	for _, formatName := range processorOrder {
		if processor, exists := r.processors[formatName]; exists {
			if processor.CanProcess(data) {
				return processor
			}
		}
	}

	// Fallback to generic processor
	return r.processors["generic"]
}

// ListProcessors returns all registered processor names
func (r *ProcessorRegistry) ListProcessors() []string {
	names := make([]string, 0, len(r.processors))
	for name := range r.processors {
		names = append(names, name)
	}
	return names
}
