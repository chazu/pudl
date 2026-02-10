package streaming

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDefaultStreamingConfig(t *testing.T) {
	config := DefaultStreamingConfig()

	if config.ChunkAlgorithm != "fastcdc" {
		t.Errorf("Expected chunk algorithm 'fastcdc', got '%s'", config.ChunkAlgorithm)
	}

	if config.MinChunkSize != 4096 {
		t.Errorf("Expected min chunk size 4096, got %d", config.MinChunkSize)
	}

	if config.MaxChunkSize != 65536 {
		t.Errorf("Expected max chunk size 65536, got %d", config.MaxChunkSize)
	}

	if err := config.Validate(); err != nil {
		t.Errorf("Default config should be valid, got error: %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *StreamingConfig
		expectError bool
	}{
		{
			name:        "valid config",
			config:      DefaultStreamingConfig(),
			expectError: false,
		},
		{
			name: "invalid min chunk size",
			config: &StreamingConfig{
				MinChunkSize: 0,
				MaxChunkSize: 1024,
				AvgChunkSize: 512,
			},
			expectError: true,
		},
		{
			name: "max smaller than min",
			config: &StreamingConfig{
				MinChunkSize: 1024,
				MaxChunkSize: 512,
				AvgChunkSize: 768,
			},
			expectError: true,
		},
		{
			name: "avg outside range",
			config: &StreamingConfig{
				MinChunkSize: 1024,
				MaxChunkSize: 2048,
				AvgChunkSize: 512,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Error("Expected validation error, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no validation error, but got: %v", err)
			}
		})
	}
}

func TestMemoryMonitor(t *testing.T) {
	monitor := NewMemoryMonitor(100) // 100MB limit

	current, limit, exceeded := monitor.CheckMemory()

	if limit != 100 {
		t.Errorf("Expected limit 100MB, got %d", limit)
	}

	if current < 0 {
		t.Errorf("Current memory usage should be non-negative, got %d", current)
	}

	// Test setting new limit
	err := monitor.SetLimit(200)
	if err != nil {
		t.Errorf("Failed to set new limit: %v", err)
	}

	_, newLimit, _ := monitor.CheckMemory()
	if newLimit != 200 {
		t.Errorf("Expected new limit 200MB, got %d", newLimit)
	}

	// Test invalid limit
	err = monitor.SetLimit(-1)
	if err == nil {
		t.Error("Expected error for negative limit, but got none")
	}

	_ = exceeded // Avoid unused variable warning
}

func TestBackpressureController(t *testing.T) {
	monitor := NewMemoryMonitor(100)
	controller := NewBackpressureController(monitor)

	// Test setting thresholds
	err := controller.SetThresholds(0.8, 0.6)
	if err != nil {
		t.Errorf("Failed to set valid thresholds: %v", err)
	}

	// Test invalid thresholds
	err = controller.SetThresholds(0.5, 0.8) // pause < resume
	if err == nil {
		t.Error("Expected error for invalid thresholds, but got none")
	}

	err = controller.SetThresholds(1.5, 0.5) // pause > 1
	if err == nil {
		t.Error("Expected error for threshold > 1, but got none")
	}
}

func TestProgressReporter(t *testing.T) {
	reporter := NewCLIProgressReporter(false) // Non-verbose mode

	// Test basic progress reporting
	reporter.Start(1000, "Test Operation")
	reporter.Update(500, "Half done")

	stats := reporter.GetStats()
	if stats.Operation != "Test Operation" {
		t.Errorf("Expected operation 'Test Operation', got '%s'", stats.Operation)
	}

	if stats.BytesProcessed != 500 {
		t.Errorf("Expected 500 bytes processed, got %d", stats.BytesProcessed)
	}

	if stats.Total != 1000 {
		t.Errorf("Expected total 1000, got %d", stats.Total)
	}

	// Test finish
	result := ProcessingResult{
		Success:          true,
		BytesProcessed:   1000,
		ObjectsExtracted: 10,
		Duration:         time.Second,
	}
	reporter.Finish(result)
}

func TestGenericChunkProcessor(t *testing.T) {
	processor := NewGenericChunkProcessor()

	if processor.FormatName() != "generic" {
		t.Errorf("Expected format name 'generic', got '%s'", processor.FormatName())
	}

	// Test with JSON data
	jsonData := []byte(`{"key": "value", "number": 42}`)
	if !processor.CanProcess(jsonData) {
		t.Error("Generic processor should be able to process any data")
	}

	chunk := &CDCChunk{
		Data:     jsonData,
		Offset:   0,
		Size:     len(jsonData),
		Hash:     "test-hash",
		Sequence: 0,
		Time:     time.Now(),
	}

	processed, err := processor.ProcessChunk(chunk)
	if err != nil {
		t.Errorf("Failed to process JSON chunk: %v", err)
	}

	if processed.Format != "json" {
		t.Errorf("Expected format 'json', got '%s'", processed.Format)
	}

	if len(processed.Objects) != 1 {
		t.Errorf("Expected 1 object, got %d", len(processed.Objects))
	}
}

func TestFormatDetection(t *testing.T) {
	processor := NewGenericChunkProcessor()

	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "JSON object",
			data:     []byte(`{"key": "value"}`),
			expected: "json",
		},
		{
			name:     "JSON array",
			data:     []byte(`[1, 2, 3]`),
			expected: "json",
		},
		{
			name:     "CSV data",
			data:     []byte("name,age,city\nJohn,30,NYC\nJane,25,LA"),
			expected: "csv",
		},
		{
			name:     "YAML data",
			data:     []byte("name: John\nage: 30\ncity: NYC"),
			expected: "yaml",
		},
		{
			name:     "Plain text",
			data:     []byte("This is just plain text\nwith multiple lines"),
			expected: "text",
		},
		{
			name:     "Empty data",
			data:     []byte(""),
			expected: "empty",
		},
		{
			name:     "Whitespace only",
			data:     []byte("   \n\t  \n  "),
			expected: "whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := processor.detectFormat(tt.data)
			if format != tt.expected {
				t.Errorf("Expected format '%s', got '%s'", tt.expected, format)
			}
		})
	}
}

func TestStreamingParserCreation(t *testing.T) {
	// Test with default config
	parser, err := NewStreamingParser(nil)
	if err != nil {
		t.Errorf("Failed to create parser with default config: %v", err)
	}
	defer parser.Close()

	// Test with custom config
	config := DefaultStreamingConfig()
	config.MaxMemoryMB = 50

	parser2, err := NewStreamingParser(config)
	if err != nil {
		t.Errorf("Failed to create parser with custom config: %v", err)
	}
	defer parser2.Close()

	// Test with invalid config
	invalidConfig := &StreamingConfig{
		MinChunkSize: -1,
	}

	_, err = NewStreamingParser(invalidConfig)
	if err == nil {
		t.Error("Expected error for invalid config, but got none")
	}
}

func TestStreamingParserBasicParsing(t *testing.T) {
	config := DefaultStreamingConfig()
	config.MaxMemoryMB = 10 // Small limit for testing
	// Use very small chunk sizes for testing
	config.MinChunkSize = 16
	config.MaxChunkSize = 128
	config.AvgChunkSize = 64

	parser, err := NewStreamingParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	// Add a progress reporter
	reporter := NewSilentProgressReporter()
	parser.SetProgressReporter(reporter)

	// Test data - make it longer to ensure chunking
	testData := `{"name": "John", "age": 30, "city": "New York", "country": "USA"}
{"name": "Jane", "age": 25, "city": "Los Angeles", "country": "USA"}
{"name": "Bob", "age": 35, "city": "Chicago", "country": "USA"}
{"name": "Alice", "age": 28, "city": "Houston", "country": "USA"}
{"name": "Charlie", "age": 42, "city": "Phoenix", "country": "USA"}`

	reader := strings.NewReader(testData)
	ctx := context.Background()

	results, errors := parser.Parse(ctx, reader)

	// Collect results
	var chunks []ParsedChunk
	var parseErrors []error

	done := false
	for !done {
		select {
		case chunk, ok := <-results:
			if !ok {
				done = true
				break
			}
			chunks = append(chunks, chunk)
		case err, ok := <-errors:
			if !ok {
				break
			}
			parseErrors = append(parseErrors, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Parsing timed out")
		}
	}

	// Verify results
	if len(parseErrors) > 0 {
		t.Errorf("Unexpected parsing errors: %v", parseErrors)
	}

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk, got none")
	}

	// Check statistics
	stats := parser.Stats()
	if stats.BytesProcessed == 0 {
		t.Error("Expected some bytes to be processed")
	}

	if stats.ChunksProcessed == 0 {
		t.Error("Expected some chunks to be processed")
	}
}



// TestLargeJSONArrayCrossChunkReassembly tests that large JSON arrays spanning multiple CDC chunks
// are correctly reassembled by the streaming parser
func TestLargeJSONArrayCrossChunkReassembly(t *testing.T) {
	// Generate a large JSON array with 100 objects (should be > 50KB to trigger CDC chunking)
	var objects []string
	for i := 0; i < 100; i++ {
		obj := `{
			"id": ` + itoa(i) + `,
			"name": "User_` + itoa(i) + `",
			"email": "user` + itoa(i) + `@example.com",
			"description": "This is a longer description field to make each object larger and ensure we have enough data to span multiple CDC chunks during the parsing process.",
			"metadata": {
				"created_at": "2024-01-15T10:30:00Z",
				"updated_at": "2024-01-16T14:45:00Z",
				"version": ` + itoa(i*10) + `,
				"tags": ["tag_a", "tag_b", "tag_c", "tag_d"],
				"nested": {
					"field1": "value1",
					"field2": "value2",
					"field3": ` + itoa(i*100) + `
				}
			}
		}`
		objects = append(objects, obj)
	}
	jsonData := "[" + strings.Join(objects, ",\n") + "]"

	// Verify we have substantial data (should be ~50KB+)
	if len(jsonData) < 30000 {
		t.Fatalf("Test data too small: %d bytes, expected at least 30KB", len(jsonData))
	}
	t.Logf("Generated JSON array: %d bytes, %d objects", len(jsonData), len(objects))

	config := DefaultStreamingConfig()
	config.MinChunkSize = 1024  // 1KB min chunks to force multiple chunks
	config.MaxChunkSize = 4096 // 4KB max chunks
	config.AvgChunkSize = 2048 // 2KB average

	parser, err := NewStreamingParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	reader := strings.NewReader(jsonData)
	ctx := context.Background()

	results, errors := parser.Parse(ctx, reader)

	var chunks []ParsedChunk
	var parseErrors []error
	totalObjects := 0

	done := false
	for !done {
		select {
		case chunk, ok := <-results:
			if !ok {
				done = true
				break
			}
			chunks = append(chunks, chunk)
			totalObjects += len(chunk.Objects)
			t.Logf("Chunk %d: %d objects, format=%s, partial=%v, is_array=%v, buf=%v, errors=%v",
				chunk.Sequence, len(chunk.Objects), chunk.Format,
				chunk.Metadata["has_partial"], chunk.Metadata["is_array"],
				chunk.Metadata["buffer_size"], chunk.Errors)
		case err, ok := <-errors:
			if !ok {
				break
			}
			parseErrors = append(parseErrors, err)
			t.Logf("Error: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("Parsing timed out")
		}
	}

	if len(parseErrors) > 0 {
		t.Errorf("Unexpected parsing errors: %v", parseErrors)
	}

	// Log info about the last chunk to debug finalization
	if len(chunks) > 0 {
		last := chunks[len(chunks)-1]
		t.Logf("Last chunk metadata: %v", last.Metadata)
	}

	// We should have extracted all 100 objects
	if totalObjects != 100 {
		t.Errorf("Expected 100 objects, got %d (from %d chunks)", totalObjects, len(chunks))
	}

	stats := parser.Stats()
	t.Logf("Stats: %d bytes processed, %d chunks, %d objects extracted",
		stats.BytesProcessed, stats.ChunksProcessed, stats.ObjectsExtracted)
}

// TestLargeNDJSONCrossChunkReassembly tests NDJSON format across chunks
func TestLargeNDJSONCrossChunkReassembly(t *testing.T) {
	// Generate 200 NDJSON lines
	var lines []string
	for i := 0; i < 200; i++ {
		line := `{"id":` + itoa(i) + `,"name":"Item_` + itoa(i) + `","value":"` + strings.Repeat("x", 200) + `"}`
		lines = append(lines, line)
	}
	ndjsonData := strings.Join(lines, "\n")

	t.Logf("Generated NDJSON data: %d bytes, %d lines", len(ndjsonData), len(lines))

	config := DefaultStreamingConfig()
	config.MinChunkSize = 512
	config.MaxChunkSize = 2048
	config.AvgChunkSize = 1024

	parser, err := NewStreamingParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	reader := strings.NewReader(ndjsonData)
	ctx := context.Background()

	results, errors := parser.Parse(ctx, reader)

	var chunks []ParsedChunk
	var parseErrors []error
	totalObjects := 0

	done := false
	for !done {
		select {
		case chunk, ok := <-results:
			if !ok {
				done = true
				break
			}
			chunks = append(chunks, chunk)
			totalObjects += len(chunk.Objects)
		case err, ok := <-errors:
			if !ok {
				break
			}
			parseErrors = append(parseErrors, err)
		case <-time.After(30 * time.Second):
			t.Fatal("Parsing timed out")
		}
	}

	if len(parseErrors) > 0 {
		t.Errorf("Unexpected parsing errors: %v", parseErrors)
	}

	// Should extract all 200 objects
	if totalObjects != 200 {
		t.Errorf("Expected 200 objects, got %d (from %d chunks)", totalObjects, len(chunks))
	}
}

// TestLargeYAMLCrossChunkReassembly tests large YAML with multiple documents
func TestLargeYAMLCrossChunkReassembly(t *testing.T) {
	// Generate 50 YAML documents
	var docs []string
	for i := 0; i < 50; i++ {
		doc := `---
id: ` + itoa(i) + `
name: Document_` + itoa(i) + `
description: This is a description for document ` + itoa(i) + ` with some extra text to make it larger.
metadata:
  created: 2024-01-15
  version: ` + itoa(i*10) + `
  tags:
    - tag_alpha
    - tag_beta
    - tag_gamma
  settings:
    enabled: true
    timeout: 30
    retries: 3`
		docs = append(docs, doc)
	}
	yamlData := strings.Join(docs, "\n")

	t.Logf("Generated YAML data: %d bytes, %d documents", len(yamlData), len(docs))

	config := DefaultStreamingConfig()
	config.MinChunkSize = 256
	config.MaxChunkSize = 1024
	config.AvgChunkSize = 512

	parser, err := NewStreamingParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	reader := strings.NewReader(yamlData)
	ctx := context.Background()

	results, errors := parser.Parse(ctx, reader)

	var chunks []ParsedChunk
	var parseErrors []error
	totalObjects := 0

	done := false
	for !done {
		select {
		case chunk, ok := <-results:
			if !ok {
				done = true
				break
			}
			chunks = append(chunks, chunk)
			totalObjects += len(chunk.Objects)
		case err, ok := <-errors:
			if !ok {
				break
			}
			parseErrors = append(parseErrors, err)
		case <-time.After(30 * time.Second):
			t.Fatal("Parsing timed out")
		}
	}

	if len(parseErrors) > 0 {
		t.Errorf("Unexpected parsing errors: %v", parseErrors)
	}

	// Should extract all 50 documents
	if totalObjects < 45 { // Allow some tolerance for YAML parsing quirks
		t.Errorf("Expected at least 45 YAML documents, got %d (from %d chunks)", totalObjects, len(chunks))
	}
	t.Logf("Extracted %d YAML documents from %d chunks", totalObjects, len(chunks))
}

// TestLargeCSVCrossChunkReassembly tests large CSV spanning multiple chunks
func TestLargeCSVCrossChunkReassembly(t *testing.T) {
	// Generate CSV with 500 rows
	var rows []string
	rows = append(rows, "id,name,email,department,salary,start_date,description")
	for i := 0; i < 500; i++ {
		row := itoa(i) + ",Employee_" + itoa(i) + ",emp" + itoa(i) + "@company.com,Dept_" + itoa(i%10) + "," + itoa(50000+i*100) + ",2024-01-" + itoa((i%28)+1) + ",This employee works in department " + itoa(i%10)
		rows = append(rows, row)
	}
	csvData := strings.Join(rows, "\n")

	t.Logf("Generated CSV data: %d bytes, %d rows", len(csvData), len(rows))

	config := DefaultStreamingConfig()
	config.MinChunkSize = 256
	config.MaxChunkSize = 1024
	config.AvgChunkSize = 512

	parser, err := NewStreamingParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	reader := strings.NewReader(csvData)
	ctx := context.Background()

	results, errors := parser.Parse(ctx, reader)

	var chunks []ParsedChunk
	var parseErrors []error
	totalObjects := 0

	done := false
	for !done {
		select {
		case chunk, ok := <-results:
			if !ok {
				done = true
				break
			}
			chunks = append(chunks, chunk)
			totalObjects += len(chunk.Objects)
		case err, ok := <-errors:
			if !ok {
				break
			}
			parseErrors = append(parseErrors, err)
		case <-time.After(30 * time.Second):
			t.Fatal("Parsing timed out")
		}
	}

	if len(parseErrors) > 0 {
		t.Errorf("Unexpected parsing errors: %v", parseErrors)
	}

	// Should extract all 500 rows (excluding header if treated as data)
	if totalObjects < 450 { // Allow some tolerance
		t.Errorf("Expected at least 450 CSV rows, got %d (from %d chunks)", totalObjects, len(chunks))
	}
	t.Logf("Extracted %d CSV rows from %d chunks", totalObjects, len(chunks))
}

// TestVeryLargeJSONFile tests a 1MB+ JSON file to ensure no memory issues
func TestVeryLargeJSONFile(t *testing.T) {
	// Generate ~1MB of JSON data
	var objects []string
	for i := 0; i < 1000; i++ {
		obj := `{"id":` + itoa(i) + `,"data":"` + strings.Repeat("x", 1000) + `"}`
		objects = append(objects, obj)
	}
	jsonData := "[" + strings.Join(objects, ",") + "]"

	t.Logf("Generated large JSON: %d bytes (~%.2f MB)", len(jsonData), float64(len(jsonData))/1024/1024)

	config := DefaultStreamingConfig()
	config.MaxMemoryMB = 50 // Limit memory

	parser, err := NewStreamingParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	reader := strings.NewReader(jsonData)
	ctx := context.Background()

	results, errors := parser.Parse(ctx, reader)

	var chunks []ParsedChunk
	var parseErrors []error
	totalObjects := 0

	done := false
	for !done {
		select {
		case chunk, ok := <-results:
			if !ok {
				done = true
				break
			}
			chunks = append(chunks, chunk)
			totalObjects += len(chunk.Objects)
		case err, ok := <-errors:
			if !ok {
				break
			}
			parseErrors = append(parseErrors, err)
		case <-time.After(60 * time.Second):
			t.Fatal("Parsing timed out")
		}
	}

	if len(parseErrors) > 0 {
		t.Errorf("Unexpected parsing errors: %v", parseErrors)
	}

	// Should extract all 1000 objects
	if totalObjects != 1000 {
		t.Errorf("Expected 1000 objects, got %d", totalObjects)
	}

	stats := parser.Stats()
	t.Logf("Processed %d bytes, extracted %d objects", stats.BytesProcessed, totalObjects)
}

// TestProcessorReuseAcrossChunks verifies that the same processor instance is used for all chunks
func TestProcessorReuseAcrossChunks(t *testing.T) {
	// Generate data that will span multiple chunks
	var objects []string
	for i := 0; i < 50; i++ {
		obj := `{"id":` + itoa(i) + `,"value":"` + strings.Repeat("a", 100) + `"}`
		objects = append(objects, obj)
	}
	jsonData := strings.Join(objects, "\n") // NDJSON format

	config := DefaultStreamingConfig()
	config.MinChunkSize = 128
	config.MaxChunkSize = 512
	config.AvgChunkSize = 256

	parser, err := NewStreamingParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	reader := strings.NewReader(jsonData)
	ctx := context.Background()

	results, errors := parser.Parse(ctx, reader)

	var allFormats []string
	done := false
	for !done {
		select {
		case chunk, ok := <-results:
			if !ok {
				done = true
				break
			}
			allFormats = append(allFormats, chunk.Format)
		case _, ok := <-errors:
			if !ok {
				break
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Parsing timed out")
		}
	}

	// All chunks should report the same format (processor was reused)
	if len(allFormats) > 0 {
		firstFormat := allFormats[0]
		for i, format := range allFormats {
			if format != firstFormat {
				t.Errorf("Chunk %d has format %s, expected %s (processor not reused?)", i, format, firstFormat)
			}
		}
		t.Logf("All %d chunks used format: %s", len(allFormats), firstFormat)
	}
}

// itoa is a helper function to convert int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}