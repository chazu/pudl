package streaming

import (
	"testing"
	"time"
)

func TestJSONChunkProcessor(t *testing.T) {
	processor := NewJSONChunkProcessor()

	// Test JSON object
	jsonData := []byte(`{"name": "John", "age": 30}`)
	chunk := &CDCChunk{
		Data:     jsonData,
		Offset:   0,
		Size:     len(jsonData),
		Hash:     "test-hash",
		Sequence: 0,
		Time:     time.Now(),
	}

	if !processor.CanProcess(jsonData) {
		t.Error("JSON processor should be able to process JSON data")
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

	// Test newline-delimited JSON
	ndjsonData := []byte(`{"name": "John", "age": 30}
{"name": "Jane", "age": 25}`)
	
	chunk.Data = ndjsonData
	chunk.Size = len(ndjsonData)

	processed, err = processor.ProcessChunk(chunk)
	if err != nil {
		t.Errorf("Failed to process NDJSON chunk: %v", err)
	}

	if len(processed.Objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(processed.Objects))
	}
}

func TestCSVChunkProcessor(t *testing.T) {
	processor := NewCSVChunkProcessor()

	// Test CSV data with headers
	csvData := []byte(`name,age,city
John,30,NYC
Jane,25,LA`)
	
	chunk := &CDCChunk{
		Data:     csvData,
		Offset:   0,
		Size:     len(csvData),
		Hash:     "test-hash",
		Sequence: 0,
		Time:     time.Now(),
	}

	if !processor.CanProcess(csvData) {
		t.Error("CSV processor should be able to process CSV data")
	}

	processed, err := processor.ProcessChunk(chunk)
	if err != nil {
		t.Errorf("Failed to process CSV chunk: %v", err)
	}

	if processed.Format != "csv" {
		t.Errorf("Expected format 'csv', got '%s'", processed.Format)
	}

	// Should have 2 data rows (excluding header)
	if len(processed.Objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(processed.Objects))
	}

	// Check if headers were detected
	headers := processor.GetHeaders()
	if headers == nil {
		t.Error("Headers should have been detected")
	}

	if len(headers) != 3 {
		t.Errorf("Expected 3 headers, got %d", len(headers))
	}

	// Check first object
	firstObj, ok := processed.Objects[0].(map[string]interface{})
	if !ok {
		t.Error("Object should be a map")
	}

	if firstObj["name"] != "John" {
		t.Errorf("Expected name 'John', got %v", firstObj["name"])
	}

	if firstObj["age"] != int64(30) {
		t.Errorf("Expected age 30, got %v", firstObj["age"])
	}
}

func TestYAMLChunkProcessor(t *testing.T) {
	processor := NewYAMLChunkProcessor()

	// Test YAML data
	yamlData := []byte(`name: John
age: 30
city: NYC`)
	
	chunk := &CDCChunk{
		Data:     yamlData,
		Offset:   0,
		Size:     len(yamlData),
		Hash:     "test-hash",
		Sequence: 0,
		Time:     time.Now(),
	}

	if !processor.CanProcess(yamlData) {
		t.Error("YAML processor should be able to process YAML data")
	}

	processed, err := processor.ProcessChunk(chunk)
	if err != nil {
		t.Errorf("Failed to process YAML chunk: %v", err)
	}

	if processed.Format != "yaml" {
		t.Errorf("Expected format 'yaml', got '%s'", processed.Format)
	}

	if len(processed.Objects) != 1 {
		t.Errorf("Expected 1 object, got %d", len(processed.Objects))
	}

	// Test multi-document YAML
	multiYamlData := []byte(`---
name: John
age: 30
---
name: Jane
age: 25`)
	
	chunk.Data = multiYamlData
	chunk.Size = len(multiYamlData)

	processed, err = processor.ProcessChunk(chunk)
	if err != nil {
		t.Errorf("Failed to process multi-document YAML chunk: %v", err)
	}

	if len(processed.Objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(processed.Objects))
	}
}

func TestProcessorRegistry(t *testing.T) {
	registry := NewProcessorRegistry()

	// Test that all processors are registered
	processors := registry.ListProcessors()
	expectedProcessors := []string{"json", "csv", "yaml", "generic"}

	if len(processors) != len(expectedProcessors) {
		t.Errorf("Expected %d processors, got %d", len(expectedProcessors), len(processors))
	}

	for _, expected := range expectedProcessors {
		found := false
		for _, actual := range processors {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected processor '%s' not found", expected)
		}
	}

	// Test getting best processor for different data types
	jsonData := []byte(`{"key": "value"}`)
	processor := registry.GetBestProcessor(jsonData)
	if processor.FormatName() != "json" {
		t.Errorf("Expected JSON processor for JSON data, got %s", processor.FormatName())
	}

	csvData := []byte(`name,age
John,30`)
	processor = registry.GetBestProcessor(csvData)
	if processor.FormatName() != "csv" {
		t.Errorf("Expected CSV processor for CSV data, got %s", processor.FormatName())
	}

	yamlData := []byte(`name: John
age: 30`)
	processor = registry.GetBestProcessor(yamlData)
	if processor.FormatName() != "yaml" {
		t.Errorf("Expected YAML processor for YAML data, got %s", processor.FormatName())
	}

	// Test fallback to generic processor
	binaryData := []byte{0x00, 0x01, 0x02, 0x03}
	processor = registry.GetBestProcessor(binaryData)
	if processor.FormatName() != "generic" {
		t.Errorf("Expected generic processor for binary data, got %s", processor.FormatName())
	}
}

func TestJSONBoundaryFinder(t *testing.T) {
	finder := NewJSONBoundaryFinder()

	// Test simple JSON object
	data := []byte(`{"name": "John", "age": 30}{"name": "Jane"}`)
	boundary := finder.FindBoundary(data)

	// The actual boundary should be at position 27 (after the closing brace)
	expectedBoundary := len(`{"name": "John", "age": 30}`)
	if boundary != expectedBoundary {
		t.Errorf("Expected boundary at %d, got %d", expectedBoundary, boundary)
	}

	// Test nested JSON
	finder.Reset()
	nestedData := []byte(`{"user": {"name": "John", "details": {"age": 30}}}`)
	boundary = finder.FindBoundary(nestedData)

	if boundary != len(nestedData) {
		t.Errorf("Expected boundary at %d, got %d", len(nestedData), boundary)
	}
}

func TestCSVBoundaryFinder(t *testing.T) {
	finder := NewCSVBoundaryFinder()

	// Test simple CSV row
	data := []byte(`"John","30","NYC"
"Jane","25","LA"`)
	boundary := finder.FindBoundary(data)

	// The actual boundary should be at position 18 (after the newline)
	expectedBoundary := len(`"John","30","NYC"`) + 1 // +1 for newline
	if boundary != expectedBoundary {
		t.Errorf("Expected boundary at %d, got %d", expectedBoundary, boundary)
	}

	// Test CSV with quoted fields containing commas
	finder.Reset()
	quotedData := []byte(`"John, Jr.","30","New York, NY"
"Jane","25","LA"`)
	boundary = finder.FindBoundary(quotedData)

	// The actual boundary should be at position 33 (after the newline)
	expectedBoundary2 := len(`"John, Jr.","30","New York, NY"`) + 1 // +1 for newline
	if boundary != expectedBoundary2 {
		t.Errorf("Expected boundary at %d, got %d", expectedBoundary2, boundary)
	}
}
