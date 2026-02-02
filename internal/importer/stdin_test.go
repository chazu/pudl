package importer

import (
	"bytes"
	"os"
	"testing"
)

func TestDetectFormatFromContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "JSON object",
			content:  `{"key": "value"}`,
			expected: "json",
		},
		{
			name:     "JSON array",
			content:  `[{"id": 1}, {"id": 2}]`,
			expected: "json",
		},
		{
			name:     "YAML",
			content:  "key: value\nother: data",
			expected: "yaml",
		},
		{
			name:     "CSV",
			content:  "name,age,city\nJohn,30,NYC",
			expected: "csv",
		},
		{
			name:     "Plain text",
			content:  "This is just plain text",
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test-*.tmp")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(tt.content); err != nil {
				t.Fatalf("Failed to write content: %v", err)
			}
			tmpFile.Close()

			result, err := DetectFormatFromContent(tmpFile.Name())
			if err != nil {
				t.Fatalf("DetectFormatFromContent failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestReadStdinToTempFile(t *testing.T) {
	// Save original stdin
	originalStdin := os.Stdin

	// Create a pipe to simulate stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	// Replace stdin with our pipe
	os.Stdin = r

	// Write test data to the pipe
	testData := []byte(`{"test": "data"}`)
	go func() {
		w.Write(testData)
		w.Close()
	}()

	// Read from stdin to temp file
	tmpPath, err := ReadStdinToTempFile()
	if err != nil {
		t.Fatalf("ReadStdinToTempFile failed: %v", err)
	}
	defer os.Remove(tmpPath)

	// Restore original stdin
	os.Stdin = originalStdin

	// Verify the content
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	if !bytes.Equal(content, testData) {
		t.Errorf("Content mismatch. Expected %s, got %s", testData, content)
	}
}

func TestGetStdinFilename(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{
			name:     "JSON format",
			format:   "json",
			expected: "stdin.json",
		},
		{
			name:     "YAML format",
			format:   "yaml",
			expected: "stdin.yaml",
		},
		{
			name:     "CSV format",
			format:   "csv",
			expected: "stdin.csv",
		},
		{
			name:     "NDJSON format",
			format:   "ndjson",
			expected: "stdin.json",
		},
		{
			name:     "Unknown format",
			format:   "unknown",
			expected: "stdin.json",
		},
		{
			name:     "Empty format",
			format:   "",
			expected: "stdin.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetStdinFilename(tt.format)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestIsStdinAvailable(t *testing.T) {
	// When running tests normally, stdin is a terminal (not available)
	// This test just ensures the function doesn't panic
	result := IsStdinAvailable()
	if result != false {
		// In test environment, stdin should not be available
		t.Logf("IsStdinAvailable returned %v (expected false in test environment)", result)
	}
}

func TestDetectFormatFromContentEmpty(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.tmp")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	result, err := DetectFormatFromContent(tmpFile.Name())
	if err != nil {
		t.Fatalf("DetectFormatFromContent failed: %v", err)
	}

	if result != "unknown" {
		t.Errorf("Expected 'unknown' for empty file, got %s", result)
	}
}

