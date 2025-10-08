package streaming

import (
	"bytes"
	"encoding/json"
)

// JSONChunkProcessor handles JSON data with boundary-aware parsing
type JSONChunkProcessor struct {
	buffer []byte // Buffer for incomplete JSON objects
}

// NewJSONChunkProcessor creates a new JSON chunk processor
func NewJSONChunkProcessor() *JSONChunkProcessor {
	return &JSONChunkProcessor{
		buffer: make([]byte, 0),
	}
}

// ProcessChunk processes a chunk containing JSON data
func (p *JSONChunkProcessor) ProcessChunk(chunk *CDCChunk) (*ProcessedChunk, error) {
	processed := &ProcessedChunk{
		Original:   chunk,
		Format:     "json",
		Objects:    []interface{}{},
		Metadata:   make(map[string]interface{}),
		Errors:     []error{},
		Partial:    false,
		Boundaries: []int{},
	}

	// Combine buffer with new chunk data
	data := append(p.buffer, chunk.Data...)

	// Parse JSON objects from the combined data
	objects, boundaries, remaining, err := p.parseJSONObjects(data)
	if err != nil {
		processed.Errors = append(processed.Errors, err)
	}

	processed.Objects = objects
	processed.Boundaries = boundaries
	processed.Partial = len(remaining) > 0

	// Update buffer with remaining incomplete data
	p.buffer = remaining

	// Add metadata
	processed.Metadata["json_objects"] = len(objects)
	processed.Metadata["buffer_size"] = len(p.buffer)
	processed.Metadata["has_partial"] = processed.Partial

	return processed, nil
}

// CanProcess returns true if the data looks like JSON
func (p *JSONChunkProcessor) CanProcess(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}

	// Check for JSON object or array start
	first := trimmed[0]
	if first == '{' || first == '[' {
		return true
	}

	// Check for newline-delimited JSON (NDJSON)
	lines := bytes.Split(trimmed, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) > 0 && (line[0] == '{' || line[0] == '[') {
			return true
		}
	}

	return false
}

// FormatName returns the name of the format this processor handles
func (p *JSONChunkProcessor) FormatName() string {
	return "json"
}

// parseJSONObjects extracts complete JSON objects from data
func (p *JSONChunkProcessor) parseJSONObjects(data []byte) ([]interface{}, []int, []byte, error) {
	// Handle different JSON formats
	if p.isJSONArray(data) {
		return p.parseJSONArray(data)
	} else if p.isNewlineDelimitedJSON(data) {
		return p.parseNewlineDelimitedJSON(data)
	} else {
		return p.parseSingleJSON(data)
	}
}

// isJSONArray checks if data starts with a JSON array
func (p *JSONChunkProcessor) isJSONArray(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '['
}

// isNewlineDelimitedJSON checks if data contains newline-delimited JSON
func (p *JSONChunkProcessor) isNewlineDelimitedJSON(data []byte) bool {
	lines := bytes.Split(data, []byte("\n"))
	validJSONLines := 0

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			// Check if this line is a complete JSON object/array
			var obj interface{}
			if json.Unmarshal(line, &obj) == nil {
				validJSONLines++
			}
		}
	}

	// Only consider it NDJSON if there are multiple valid JSON objects on separate lines
	return validJSONLines > 1
}

// parseJSONArray parses a JSON array, handling incomplete arrays
func (p *JSONChunkProcessor) parseJSONArray(data []byte) ([]interface{}, []int, []byte, error) {
	var objects []interface{}
	var boundaries []int

	// Try to parse the entire array
	var arr []interface{}
	if err := json.Unmarshal(data, &arr); err == nil {
		// Complete array found
		objects = arr
		boundaries = []int{len(data)}
		return objects, boundaries, []byte{}, nil
	}

	// Array is incomplete, try to find complete elements
	decoder := json.NewDecoder(bytes.NewReader(data))
	offset := 0

	for decoder.More() {
		var obj interface{}
		startPos := int(decoder.InputOffset())

		if err := decoder.Decode(&obj); err != nil {
			// Incomplete object, return remaining data
			remaining := data[startPos:]
			return objects, boundaries, remaining, nil
		}

		objects = append(objects, obj)
		endPos := int(decoder.InputOffset())
		boundaries = append(boundaries, endPos-offset)
		offset = endPos
	}

	return objects, boundaries, []byte{}, nil
}

// parseNewlineDelimitedJSON parses newline-delimited JSON
func (p *JSONChunkProcessor) parseNewlineDelimitedJSON(data []byte) ([]interface{}, []int, []byte, error) {
	var objects []interface{}
	var boundaries []int
	var remaining []byte

	lines := bytes.Split(data, []byte("\n"))
	currentPos := 0

	for i, line := range lines {
		line = bytes.TrimSpace(line)

		// Skip empty lines
		if len(line) == 0 {
			currentPos += len(lines[i]) + 1 // +1 for newline
			continue
		}

		// Try to parse the line as JSON
		var obj interface{}
		if err := json.Unmarshal(line, &obj); err != nil {
			// If this is the last line and it's incomplete, save as remaining
			if i == len(lines)-1 {
				remaining = line
				break
			}
			// Otherwise, skip invalid JSON
			currentPos += len(lines[i]) + 1
			continue
		}

		objects = append(objects, obj)
		boundaries = append(boundaries, currentPos+len(line))
		currentPos += len(lines[i]) + 1
	}

	return objects, boundaries, remaining, nil
}

// parseSingleJSON parses a single JSON object
func (p *JSONChunkProcessor) parseSingleJSON(data []byte) ([]interface{}, []int, []byte, error) {
	var objects []interface{}
	var boundaries []int

	// Try to parse as a single JSON object
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		// Data is incomplete, return as remaining
		return objects, boundaries, data, err
	}

	objects = append(objects, obj)
	boundaries = append(boundaries, len(data))
	return objects, boundaries, []byte{}, nil
}

// Reset clears the internal buffer
func (p *JSONChunkProcessor) Reset() {
	p.buffer = p.buffer[:0]
}

// GetBufferSize returns the current buffer size
func (p *JSONChunkProcessor) GetBufferSize() int {
	return len(p.buffer)
}

// JSONBoundaryFinder helps find JSON object boundaries in streaming data
type JSONBoundaryFinder struct {
	braceDepth   int
	bracketDepth int
	inString     bool
	escaped      bool
}

// NewJSONBoundaryFinder creates a new JSON boundary finder
func NewJSONBoundaryFinder() *JSONBoundaryFinder {
	return &JSONBoundaryFinder{}
}

// FindBoundary finds the end of a complete JSON object in the data
func (f *JSONBoundaryFinder) FindBoundary(data []byte) int {
	for i, b := range data {
		switch {
		case f.escaped:
			f.escaped = false
			continue
		case f.inString && b == '\\':
			f.escaped = true
			continue
		case b == '"':
			f.inString = !f.inString
			continue
		case f.inString:
			continue
		case b == '{':
			f.braceDepth++
		case b == '}':
			f.braceDepth--
			if f.braceDepth == 0 && f.bracketDepth == 0 {
				return i + 1 // Include the closing brace
			}
		case b == '[':
			f.bracketDepth++
		case b == ']':
			f.bracketDepth--
			if f.braceDepth == 0 && f.bracketDepth == 0 {
				return i + 1 // Include the closing bracket
			}
		}
	}

	return -1 // No complete object found
}

// Reset resets the boundary finder state
func (f *JSONBoundaryFinder) Reset() {
	f.braceDepth = 0
	f.bracketDepth = 0
	f.inString = false
	f.escaped = false
}
