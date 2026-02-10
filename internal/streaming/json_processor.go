package streaming

import (
	"bytes"
	"encoding/json"
)

// JSONChunkProcessor handles JSON data with boundary-aware parsing
type JSONChunkProcessor struct {
	buffer         []byte              // Buffer for incomplete JSON objects
	boundaryFinder *JSONBoundaryFinder // Tracks JSON structure across chunks
	formatDetected bool                // Whether we've detected the JSON format
	isArray        bool                // Whether the root structure is an array
	isNDJSON       bool                // Whether this is newline-delimited JSON
	sequence       int                 // Chunk sequence for Finalize
}

// NewJSONChunkProcessor creates a new JSON chunk processor
func NewJSONChunkProcessor() *JSONChunkProcessor {
	return &JSONChunkProcessor{
		buffer:         make([]byte, 0),
		boundaryFinder: NewJSONBoundaryFinder(),
		formatDetected: false,
		isArray:        false,
		isNDJSON:       false,
		sequence:       0,
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

	// Detect format on first chunk with data
	if !p.formatDetected && len(bytes.TrimSpace(data)) > 0 {
		p.detectFormat(data)
	}

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
	p.sequence = chunk.Sequence

	// Add metadata
	processed.Metadata["json_objects"] = len(objects)
	processed.Metadata["buffer_size"] = len(p.buffer)
	processed.Metadata["has_partial"] = processed.Partial
	processed.Metadata["is_array"] = p.isArray
	processed.Metadata["is_ndjson"] = p.isNDJSON

	return processed, nil
}

// detectFormat detects whether this is a JSON array, object, or NDJSON
func (p *JSONChunkProcessor) detectFormat(data []byte) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return
	}

	p.formatDetected = true

	// Check for array
	if trimmed[0] == '[' {
		p.isArray = true
		return
	}

	// Check for NDJSON (multiple JSON objects separated by newlines)
	// NDJSON format: each line is a complete JSON object/array with NO leading whitespace
	if trimmed[0] == '{' {
		// Look for lines that start with { or [ at the very beginning (no leading whitespace)
		// This distinguishes NDJSON from a formatted single JSON object with nested content
		lines := bytes.Split(trimmed, []byte("\n"))
		rootLevelJSONLines := 0
		for _, line := range lines {
			// For NDJSON, each root-level object starts at column 0 (no leading whitespace)
			if len(line) > 0 && (line[0] == '{' || line[0] == '[') {
				rootLevelJSONLines++
			}
		}
		p.isNDJSON = rootLevelJSONLines > 1
	}
}

// Finalize flushes any remaining buffered data at end of stream
func (p *JSONChunkProcessor) Finalize() (*ProcessedChunk, error) {
	if len(p.buffer) == 0 {
		return nil, nil
	}

	// Try to parse whatever is left in the buffer
	trimmed := bytes.TrimSpace(p.buffer)
	if len(trimmed) == 0 {
		p.buffer = nil
		return nil, nil
	}

	processed := &ProcessedChunk{
		Original: &CDCChunk{
			Data:     p.buffer,
			Offset:   0,
			Size:     len(p.buffer),
			Hash:     "",
			Sequence: p.sequence + 1,
		},
		Format:     "json",
		Objects:    []interface{}{},
		Metadata:   make(map[string]interface{}),
		Errors:     []error{},
		Partial:    false,
		Boundaries: []int{},
	}

	// Use the same parsing logic as ProcessChunk based on detected format
	if p.isArray {
		objects, boundaries, _, err := p.parseJSONArray(trimmed)
		if err != nil {
			processed.Errors = append(processed.Errors, err)
		}
		processed.Objects = objects
		processed.Boundaries = boundaries
	} else if p.isNDJSON {
		objects, boundaries, _, err := p.parseNewlineDelimitedJSON(trimmed)
		if err != nil {
			processed.Errors = append(processed.Errors, err)
		}
		processed.Objects = objects
		processed.Boundaries = boundaries
	} else {
		// Try to parse as a single JSON object
		var obj interface{}
		if err := json.Unmarshal(trimmed, &obj); err == nil {
			processed.Objects = append(processed.Objects, obj)
			processed.Boundaries = append(processed.Boundaries, len(trimmed))
		} else {
			// If it fails to parse, include as error but don't fail completely
			processed.Errors = append(processed.Errors, err)
		}
	}

	processed.Metadata["finalized"] = true
	processed.Metadata["buffer_size"] = len(p.buffer)
	processed.Metadata["is_array"] = p.isArray
	processed.Metadata["is_ndjson"] = p.isNDJSON

	// Clear the buffer
	p.buffer = nil

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
	// Use detected format from first chunk rather than checking current data
	// This is crucial for cross-chunk reassembly where later chunks don't start with [
	if p.isArray {
		return p.parseJSONArray(data)
	} else if p.isNDJSON {
		return p.parseNewlineDelimitedJSON(data)
	} else {
		// Fall back to content-based detection if format not detected yet
		if p.isJSONArray(data) {
			return p.parseJSONArray(data)
		} else if p.isNewlineDelimitedJSON(data) {
			return p.parseNewlineDelimitedJSON(data)
		}
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

// parseJSONArray parses a JSON array, handling incomplete arrays that span chunks
func (p *JSONChunkProcessor) parseJSONArray(data []byte) ([]interface{}, []int, []byte, error) {
	var objects []interface{}
	var boundaries []int

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return objects, boundaries, data, nil
	}

	// Try to parse the entire array first (fast path)
	var arr []interface{}
	if err := json.Unmarshal(trimmed, &arr); err == nil {
		// Complete array found
		objects = arr
		boundaries = []int{len(data)}
		return objects, boundaries, []byte{}, nil
	}

	// Array is incomplete - parse individual elements
	// Find where to start parsing (skip opening bracket if present)
	startIdx := 0
	for i, b := range trimmed {
		if b == '[' {
			startIdx = i + 1
			break
		} else if b == '{' || b == '"' || (b >= '0' && b <= '9') || b == '-' || b == 't' || b == 'f' || b == 'n' {
			// Data starts with an element (continuation from previous chunk)
			startIdx = i
			break
		} else if b == ',' {
			// Skip commas between elements
			startIdx = i + 1
			continue
		}
	}

	// Parse individual objects from the array
	remaining := trimmed[startIdx:]
	for len(remaining) > 0 {
		// Skip whitespace and commas
		idx := 0
		for idx < len(remaining) {
			b := remaining[idx]
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == ',' {
				idx++
			} else {
				break
			}
		}
		remaining = remaining[idx:]

		if len(remaining) == 0 {
			break
		}

		// Check for array end
		if remaining[0] == ']' {
			// End of array, return empty remaining
			return objects, boundaries, []byte{}, nil
		}

		// Try to find a complete object using the boundary finder
		finder := NewJSONBoundaryFinder()
		boundary := finder.FindBoundary(remaining)

		if boundary == -1 {
			// No complete object found, return remaining as buffer
			return objects, boundaries, remaining, nil
		}

		// Parse the complete object
		objData := remaining[:boundary]
		var obj interface{}
		if err := json.Unmarshal(objData, &obj); err != nil {
			// Parse error - might be corrupted or truly incomplete
			return objects, boundaries, remaining, nil
		}

		objects = append(objects, obj)
		boundaries = append(boundaries, boundary)
		remaining = remaining[boundary:]
	}

	return objects, boundaries, remaining, nil
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

// Reset clears the internal buffer and state for reuse
func (p *JSONChunkProcessor) Reset() {
	p.buffer = p.buffer[:0]
	p.boundaryFinder.Reset()
	p.formatDetected = false
	p.isArray = false
	p.isNDJSON = false
	p.sequence = 0
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
