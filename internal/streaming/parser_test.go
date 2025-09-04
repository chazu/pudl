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
