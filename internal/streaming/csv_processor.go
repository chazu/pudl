package streaming

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CSVChunkProcessor handles CSV data with row completion logic
type CSVChunkProcessor struct {
	buffer  []byte   // Buffer for incomplete CSV rows
	headers []string // CSV headers if detected
}

// NewCSVChunkProcessor creates a new CSV chunk processor
func NewCSVChunkProcessor() *CSVChunkProcessor {
	return &CSVChunkProcessor{
		buffer:  make([]byte, 0),
		headers: nil,
	}
}

// ProcessChunk processes a chunk containing CSV data
func (p *CSVChunkProcessor) ProcessChunk(chunk *CDCChunk) (*ProcessedChunk, error) {
	processed := &ProcessedChunk{
		Original:   chunk,
		Format:     "csv",
		Objects:    []interface{}{},
		Metadata:   make(map[string]interface{}),
		Errors:     []error{},
		Partial:    false,
		Boundaries: []int{},
	}

	// Combine buffer with new chunk data
	data := append(p.buffer, chunk.Data...)
	
	// Parse CSV rows from the combined data
	rows, boundaries, remaining, err := p.parseCSVRows(data)
	if err != nil {
		processed.Errors = append(processed.Errors, err)
	}

	// Convert rows to objects
	objects := p.rowsToObjects(rows)
	processed.Objects = objects
	processed.Boundaries = boundaries
	processed.Partial = len(remaining) > 0

	// Update buffer with remaining incomplete data
	p.buffer = remaining

	// Add metadata
	processed.Metadata["csv_rows"] = len(rows)
	processed.Metadata["csv_columns"] = p.getColumnCount(rows)
	processed.Metadata["has_headers"] = p.headers != nil
	processed.Metadata["buffer_size"] = len(p.buffer)
	processed.Metadata["has_partial"] = processed.Partial

	if p.headers != nil {
		processed.Metadata["headers"] = p.headers
	}

	return processed, nil
}

// CanProcess returns true if the data looks like CSV
func (p *CSVChunkProcessor) CanProcess(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}

	lines := bytes.Split(trimmed, []byte("\n"))
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

	return consistentLines >= 1
}

// FormatName returns the name of the format this processor handles
func (p *CSVChunkProcessor) FormatName() string {
	return "csv"
}

// parseCSVRows extracts complete CSV rows from data
func (p *CSVChunkProcessor) parseCSVRows(data []byte) ([][]string, []int, []byte, error) {
	var rows [][]string
	var boundaries []int
	var remaining []byte

	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1 // Allow variable number of fields
	reader.TrimLeadingSpace = true

	currentPos := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Check if this is due to incomplete data at the end
			if strings.Contains(err.Error(), "bare") || strings.Contains(err.Error(), "quote") {
				// Find the start of the incomplete row
				lines := bytes.Split(data, []byte("\n"))
				if len(lines) > 0 {
					// The last line might be incomplete
					lastLineStart := len(data) - len(lines[len(lines)-1])
					if len(lines) > 1 && len(lines[len(lines)-1]) == 0 {
						// If last line is empty, take the second to last
						lastLineStart = len(data) - len(lines[len(lines)-1]) - len(lines[len(lines)-2]) - 1
					}
					remaining = data[lastLineStart:]
				}
				break
			}
			return rows, boundaries, remaining, fmt.Errorf("CSV parsing error: %w", err)
		}

		rows = append(rows, record)
		
		// Calculate boundary position
		// This is approximate since csv.Reader doesn't provide exact positions
		rowText := strings.Join(record, ",") + "\n"
		currentPos += len(rowText)
		boundaries = append(boundaries, currentPos)
	}

	// Detect headers if this is the first chunk and we have rows
	if p.headers == nil && len(rows) > 0 {
		p.detectHeaders(rows[0])
	}

	return rows, boundaries, remaining, nil
}

// detectHeaders attempts to detect if the first row contains headers
func (p *CSVChunkProcessor) detectHeaders(firstRow []string) {
	// Simple heuristic: if all fields are non-numeric and contain letters, likely headers
	hasHeaders := true
	for _, field := range firstRow {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		
		// If field is purely numeric, probably not a header
		if _, err := strconv.ParseFloat(field, 64); err == nil {
			hasHeaders = false
			break
		}
		
		// If field doesn't contain letters, probably not a header
		hasLetters := false
		for _, r := range field {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				hasLetters = true
				break
			}
		}
		if !hasLetters {
			hasHeaders = false
			break
		}
	}

	if hasHeaders {
		p.headers = make([]string, len(firstRow))
		copy(p.headers, firstRow)
	}
}

// rowsToObjects converts CSV rows to objects
func (p *CSVChunkProcessor) rowsToObjects(rows [][]string) []interface{} {
	var objects []interface{}

	for i, row := range rows {
		// Skip header row if we detected headers
		if i == 0 && p.headers != nil {
			continue
		}

		obj := make(map[string]interface{})
		
		if p.headers != nil {
			// Use headers as keys
			for j, value := range row {
				key := fmt.Sprintf("col_%d", j)
				if j < len(p.headers) {
					key = p.headers[j]
				}
				obj[key] = p.parseValue(value)
			}
		} else {
			// Use generic column names
			for j, value := range row {
				key := fmt.Sprintf("col_%d", j)
				obj[key] = p.parseValue(value)
			}
		}

		// Add row metadata
		obj["_row_number"] = i
		obj["_column_count"] = len(row)

		objects = append(objects, obj)
	}

	return objects
}

// parseValue attempts to parse a CSV value to the appropriate type
func (p *CSVChunkProcessor) parseValue(value string) interface{} {
	value = strings.TrimSpace(value)
	
	if value == "" {
		return nil
	}

	// Try to parse as integer
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal
	}

	// Try to parse as float
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}

	// Try to parse as boolean
	if boolVal, err := strconv.ParseBool(value); err == nil {
		return boolVal
	}

	// Return as string
	return value
}

// getColumnCount returns the number of columns in the CSV data
func (p *CSVChunkProcessor) getColumnCount(rows [][]string) int {
	if len(rows) == 0 {
		return 0
	}
	
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	
	return maxCols
}

// Reset clears the internal buffer and headers
func (p *CSVChunkProcessor) Reset() {
	p.buffer = p.buffer[:0]
	p.headers = nil
}

// GetBufferSize returns the current buffer size
func (p *CSVChunkProcessor) GetBufferSize() int {
	return len(p.buffer)
}

// GetHeaders returns the detected headers
func (p *CSVChunkProcessor) GetHeaders() []string {
	if p.headers == nil {
		return nil
	}
	
	headers := make([]string, len(p.headers))
	copy(headers, p.headers)
	return headers
}

// CSVBoundaryFinder helps find CSV row boundaries in streaming data
type CSVBoundaryFinder struct {
	inQuotes bool
	escaped  bool
}

// NewCSVBoundaryFinder creates a new CSV boundary finder
func NewCSVBoundaryFinder() *CSVBoundaryFinder {
	return &CSVBoundaryFinder{}
}

// FindBoundary finds the end of a complete CSV row in the data
func (f *CSVBoundaryFinder) FindBoundary(data []byte) int {
	for i, b := range data {
		switch {
		case f.escaped:
			f.escaped = false
			continue
		case f.inQuotes && b == '\\':
			f.escaped = true
			continue
		case b == '"':
			f.inQuotes = !f.inQuotes
			continue
		case f.inQuotes:
			continue
		case b == '\n':
			return i + 1 // Include the newline
		case b == '\r':
			// Handle Windows line endings
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2 // Include both \r\n
			}
			return i + 1 // Just \r
		}
	}
	
	return -1 // No complete row found
}

// Reset resets the boundary finder state
func (f *CSVBoundaryFinder) Reset() {
	f.inQuotes = false
	f.escaped = false
}
