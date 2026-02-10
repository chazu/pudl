package streaming

import (
	"bytes"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLChunkProcessor handles YAML data with document boundary detection
type YAMLChunkProcessor struct {
	buffer         []byte             // Buffer for incomplete YAML documents
	boundaryFinder *YAMLBoundaryFinder // Tracks YAML document boundaries across chunks
	formatDetected bool               // Whether we've detected the format
	hasMultipleDocs bool              // Whether stream contains multiple documents
	sequence       int                // Chunk sequence for Finalize
}

// NewYAMLChunkProcessor creates a new YAML chunk processor
func NewYAMLChunkProcessor() *YAMLChunkProcessor {
	return &YAMLChunkProcessor{
		buffer:         make([]byte, 0),
		boundaryFinder: NewYAMLBoundaryFinder(),
		formatDetected: false,
		hasMultipleDocs: false,
		sequence:       0,
	}
}

// ProcessChunk processes a chunk containing YAML data
func (p *YAMLChunkProcessor) ProcessChunk(chunk *CDCChunk) (*ProcessedChunk, error) {
	processed := &ProcessedChunk{
		Original:   chunk,
		Format:     "yaml",
		Objects:    []interface{}{},
		Metadata:   make(map[string]interface{}),
		Errors:     []error{},
		Partial:    false,
		Boundaries: []int{},
	}

	// Combine buffer with new chunk data
	data := append(p.buffer, chunk.Data...)

	// Detect format on first chunk
	if !p.formatDetected && len(bytes.TrimSpace(data)) > 0 {
		p.detectFormat(data)
	}

	// Parse YAML documents from the combined data
	documents, boundaries, remaining, err := p.parseYAMLDocuments(data)
	if err != nil {
		processed.Errors = append(processed.Errors, err)
	}

	processed.Objects = documents
	processed.Boundaries = boundaries
	processed.Partial = len(remaining) > 0

	// Update buffer with remaining incomplete data
	p.buffer = remaining
	p.sequence = chunk.Sequence

	// Add metadata
	processed.Metadata["yaml_documents"] = len(documents)
	processed.Metadata["buffer_size"] = len(p.buffer)
	processed.Metadata["has_partial"] = processed.Partial
	processed.Metadata["has_multiple_docs"] = p.hasMultipleDocs

	return processed, nil
}

// detectFormat detects whether this YAML has multiple documents
func (p *YAMLChunkProcessor) detectFormat(data []byte) {
	p.formatDetected = true
	// Check for multiple document separators
	docCount := bytes.Count(data, []byte("---"))
	p.hasMultipleDocs = docCount > 1
}

// Finalize flushes any remaining buffered data at end of stream
func (p *YAMLChunkProcessor) Finalize() (*ProcessedChunk, error) {
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
		Format:     "yaml",
		Objects:    []interface{}{},
		Metadata:   make(map[string]interface{}),
		Errors:     []error{},
		Partial:    false,
		Boundaries: []int{},
	}

	// Try to parse the remaining data as YAML
	var doc interface{}
	if err := yaml.Unmarshal(trimmed, &doc); err == nil {
		if doc != nil {
			processed.Objects = append(processed.Objects, doc)
			processed.Boundaries = append(processed.Boundaries, len(trimmed))
		}
	} else {
		// If it fails to parse, include as error
		processed.Errors = append(processed.Errors, err)
	}

	processed.Metadata["finalized"] = true
	processed.Metadata["buffer_size"] = len(p.buffer)

	// Clear the buffer
	p.buffer = nil

	return processed, nil
}

// CanProcess returns true if the data looks like YAML
func (p *YAMLChunkProcessor) CanProcess(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}

	lines := bytes.Split(trimmed, []byte("\n"))
	yamlIndicators := 0
	
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		
		// Look for YAML indicators
		if bytes.Contains(line, []byte(": ")) || 
		   bytes.HasPrefix(line, []byte("- ")) ||
		   bytes.HasPrefix(line, []byte("---")) ||
		   bytes.HasPrefix(line, []byte("...")) {
			yamlIndicators++
		}
		
		// Check for indented content (common in YAML)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			yamlIndicators++
		}
	}
	
	return yamlIndicators > 0
}

// FormatName returns the name of the format this processor handles
func (p *YAMLChunkProcessor) FormatName() string {
	return "yaml"
}

// parseYAMLDocuments extracts complete YAML documents from data
func (p *YAMLChunkProcessor) parseYAMLDocuments(data []byte) ([]interface{}, []int, []byte, error) {
	var documents []interface{}
	var boundaries []int
	var remaining []byte

	// Split by document separators
	docSeparator := regexp.MustCompile(`(?m)^---\s*$`)
	docEnd := regexp.MustCompile(`(?m)^\.\.\.?\s*$`)
	
	dataStr := string(data)
	
	// Find document boundaries
	separatorIndices := docSeparator.FindAllStringIndex(dataStr, -1)
	endIndices := docEnd.FindAllStringIndex(dataStr, -1)
	
	if len(separatorIndices) == 0 && len(endIndices) == 0 {
		// Single document without explicit separators
		return p.parseSingleYAMLDocument(data)
	}

	// Process documents with explicit separators
	for i, sepIndex := range separatorIndices {
		// Skip the separator itself
		docStart := sepIndex[1]
		
		// Find the end of this document
		var docEnd int
		if i+1 < len(separatorIndices) {
			// Next separator marks the end
			docEnd = separatorIndices[i+1][0]
		} else {
			// Check for explicit document end
			foundEnd := false
			for _, endIndex := range endIndices {
				if endIndex[0] > docStart {
					docEnd = endIndex[0]
					foundEnd = true
					break
				}
			}
			if !foundEnd {
				// Document continues to end of data
				docEnd = len(dataStr)
			}
		}
		
		// Extract document content
		docContent := strings.TrimSpace(dataStr[docStart:docEnd])
		if docContent == "" {
			continue
		}
		
		// Try to parse the document
		var doc interface{}
		if err := yaml.Unmarshal([]byte(docContent), &doc); err != nil {
			// If this is the last document and parsing fails, it might be incomplete
			if i == len(separatorIndices)-1 {
				remaining = []byte(docContent)
				break
			}
			// Skip invalid documents
			continue
		}
		
		documents = append(documents, doc)
		boundaries = append(boundaries, docEnd)
	}

	return documents, boundaries, remaining, nil
}

// parseSingleYAMLDocument parses a single YAML document
func (p *YAMLChunkProcessor) parseSingleYAMLDocument(data []byte) ([]interface{}, []int, []byte, error) {
	var documents []interface{}
	var boundaries []int

	// Try to parse as a single YAML document
	var doc interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Data might be incomplete, return as remaining
		return documents, boundaries, data, err
	}

	documents = append(documents, doc)
	boundaries = append(boundaries, len(data))
	return documents, boundaries, []byte{}, nil
}

// Reset clears the internal buffer and state for reuse
func (p *YAMLChunkProcessor) Reset() {
	p.buffer = p.buffer[:0]
	p.boundaryFinder.Reset()
	p.formatDetected = false
	p.hasMultipleDocs = false
	p.sequence = 0
}

// GetBufferSize returns the current buffer size
func (p *YAMLChunkProcessor) GetBufferSize() int {
	return len(p.buffer)
}

// YAMLBoundaryFinder helps find YAML document boundaries in streaming data
type YAMLBoundaryFinder struct {
	inDocument bool
	indentLevel int
}

// NewYAMLBoundaryFinder creates a new YAML boundary finder
func NewYAMLBoundaryFinder() *YAMLBoundaryFinder {
	return &YAMLBoundaryFinder{}
}

// FindBoundary finds the end of a complete YAML document in the data
func (f *YAMLBoundaryFinder) FindBoundary(data []byte) int {
	lines := bytes.Split(data, []byte("\n"))
	
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		
		// Skip empty lines and comments
		if len(trimmed) == 0 || trimmed[0] == '#' {
			continue
		}
		
		// Check for document separator
		if bytes.HasPrefix(trimmed, []byte("---")) {
			if f.inDocument {
				// End of current document
				return f.calculateLineOffset(lines, i)
			} else {
				// Start of new document
				f.inDocument = true
				continue
			}
		}
		
		// Check for document end marker
		if bytes.HasPrefix(trimmed, []byte("...")) {
			if f.inDocument {
				f.inDocument = false
				return f.calculateLineOffset(lines, i+1)
			}
		}
		
		// If we're in a document, track indentation
		if f.inDocument {
			indent := f.getIndentLevel(line)
			if indent == 0 && len(trimmed) > 0 {
				// Top-level content might indicate document boundary
				f.indentLevel = 0
			}
		}
	}
	
	return -1 // No complete document found
}

// calculateLineOffset calculates the byte offset for a given line
func (f *YAMLBoundaryFinder) calculateLineOffset(lines [][]byte, lineIndex int) int {
	offset := 0
	for i := 0; i < lineIndex && i < len(lines); i++ {
		offset += len(lines[i]) + 1 // +1 for newline
	}
	return offset
}

// getIndentLevel returns the indentation level of a line
func (f *YAMLBoundaryFinder) getIndentLevel(line []byte) int {
	indent := 0
	for _, b := range line {
		if b == ' ' {
			indent++
		} else if b == '\t' {
			indent += 4 // Treat tab as 4 spaces
		} else {
			break
		}
	}
	return indent
}

// Reset resets the boundary finder state
func (f *YAMLBoundaryFinder) Reset() {
	f.inDocument = false
	f.indentLevel = 0
}

// isYAMLKey checks if a line contains a YAML key-value pair
func isYAMLKey(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return false
	}
	
	// Look for key: value pattern
	colonIndex := bytes.Index(trimmed, []byte(":"))
	if colonIndex == -1 {
		return false
	}
	
	// Make sure it's not inside quotes
	inQuotes := false
	for i, b := range trimmed[:colonIndex] {
		if b == '"' || b == '\'' {
			if i == 0 || trimmed[i-1] != '\\' {
				inQuotes = !inQuotes
			}
		}
	}
	
	return !inQuotes
}

// isYAMLListItem checks if a line is a YAML list item
func isYAMLListItem(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	return bytes.HasPrefix(trimmed, []byte("- "))
}
