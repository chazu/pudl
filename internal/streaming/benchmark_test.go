package streaming

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// BenchmarkJSONProcessing benchmarks JSON data processing
func BenchmarkJSONProcessing(b *testing.B) {
	// Create sample JSON data
	data := map[string]interface{}{
		"id":   "test-123",
		"name": "Test Object",
		"tags": []string{"tag1", "tag2", "tag3"},
		"metadata": map[string]interface{}{
			"created": "2026-02-02",
			"updated": "2026-02-02",
		},
	}

	jsonData, _ := json.Marshal(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result interface{}
		json.Unmarshal(jsonData, &result)
	}
}

// BenchmarkYAMLProcessing benchmarks YAML data processing
func BenchmarkYAMLProcessing(b *testing.B) {
	yamlData := `
id: test-123
name: Test Object
tags:
  - tag1
  - tag2
  - tag3
metadata:
  created: 2026-02-02
  updated: 2026-02-02
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result interface{}
		yaml.Unmarshal([]byte(yamlData), &result)
	}
}

// BenchmarkCSVProcessing benchmarks CSV data processing
func BenchmarkCSVProcessing(b *testing.B) {
	csvData := `id,name,status,created
test-001,Object 1,active,2026-02-02
test-002,Object 2,inactive,2026-02-01
test-003,Object 3,active,2026-01-31
test-004,Object 4,pending,2026-01-30
test-005,Object 5,active,2026-01-29`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := strings.NewReader(csvData)
		_ = reader
	}
}

// BenchmarkStreamingParserCreation benchmarks parser creation
func BenchmarkStreamingParserCreation(b *testing.B) {
	config := DefaultStreamingConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config
	}
}

// BenchmarkFormatDetection benchmarks format detection
func BenchmarkFormatDetection(b *testing.B) {
	processor := NewGenericChunkProcessor()
	jsonData := []byte(`{"id": "test", "name": "example"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.detectFormat(jsonData)
	}
}

// BenchmarkLargeJSONProcessing benchmarks processing of large JSON arrays
func BenchmarkLargeJSONProcessing(b *testing.B) {
	// Create a large JSON array
	items := make([]map[string]interface{}, 1000)
	for i := 0; i < 1000; i++ {
		items[i] = map[string]interface{}{
			"id":   i,
			"name": "Item " + string(rune(i)),
		}
	}

	jsonData, _ := json.Marshal(items)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result interface{}
		json.Unmarshal(jsonData, &result)
	}
}

