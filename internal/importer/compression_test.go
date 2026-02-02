package importer

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestDetectCompression(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		content    []byte
		expected   string
	}{
		{
			name:       "gzip by extension",
			filename:   "test.json.gz",
			content:    []byte{0x1f, 0x8b, 0x08, 0x00}, // gzip magic bytes
			expected:   "gzip",
		},
		{
			name:       "zstd by extension",
			filename:   "test.json.zst",
			content:    []byte{0x28, 0xb5, 0x2f, 0xfd}, // zstd magic bytes
			expected:   "zstd",
		},
		{
			name:       "gzip by magic bytes",
			filename:   "test.data",
			content:    []byte{0x1f, 0x8b, 0x08, 0x00},
			expected:   "gzip",
		},
		{
			name:       "zstd by magic bytes",
			filename:   "test.data",
			content:    []byte{0x28, 0xb5, 0x2f, 0xfd},
			expected:   "zstd",
		},
		{
			name:       "no compression",
			filename:   "test.json",
			content:    []byte(`{"key": "value"}`),
			expected:   "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", tt.filename)
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.Write(tt.content); err != nil {
				t.Fatalf("Failed to write content: %v", err)
			}
			tmpFile.Close()

			result := DetectCompression(tmpFile.Name())
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestWrapReader(t *testing.T) {
	originalData := []byte(`{"test": "data"}`)

	tests := []struct {
		name        string
		compression string
		setupFunc   func() io.Reader
	}{
		{
			name:        "gzip reader",
			compression: "gzip",
			setupFunc: func() io.Reader {
				var buf bytes.Buffer
				w := gzip.NewWriter(&buf)
				w.Write(originalData)
				w.Close()
				return bytes.NewReader(buf.Bytes())
			},
		},
		{
			name:        "zstd reader",
			compression: "zstd",
			setupFunc: func() io.Reader {
				var buf bytes.Buffer
				w, _ := zstd.NewWriter(&buf)
				w.Write(originalData)
				w.Close()
				return bytes.NewReader(buf.Bytes())
			},
		},
		{
			name:        "no compression",
			compression: "none",
			setupFunc: func() io.Reader {
				return bytes.NewReader(originalData)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := tt.setupFunc()
			wrapped, err := WrapReader(reader, tt.compression)
			if err != nil {
				t.Fatalf("WrapReader failed: %v", err)
			}

			data, err := io.ReadAll(wrapped)
			if err != nil {
				t.Fatalf("Failed to read from wrapped reader: %v", err)
			}

			if !bytes.Equal(data, originalData) {
				t.Errorf("Data mismatch. Expected %s, got %s", originalData, data)
			}
		})
	}
}

func TestDecompressFile(t *testing.T) {
	originalData := []byte(`{"test": "data"}`)

	// Create gzip compressed file
	tmpFile, err := os.CreateTemp("", "test-*.json.gz")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	w := gzip.NewWriter(tmpFile)
	w.Write(originalData)
	w.Close()
	tmpFile.Close()

	// Decompress
	decompressed, err := DecompressFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("DecompressFile failed: %v", err)
	}
	defer os.Remove(decompressed)

	// Verify decompressed content
	data, err := os.ReadFile(decompressed)
	if err != nil {
		t.Fatalf("Failed to read decompressed file: %v", err)
	}

	if !bytes.Equal(data, originalData) {
		t.Errorf("Data mismatch. Expected %s, got %s", originalData, data)
	}
}

func TestDecompressFileUncompressed(t *testing.T) {
	originalData := []byte(`{"test": "data"}`)

	// Create uncompressed file
	tmpFile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.Write(originalData)
	tmpFile.Close()

	// Should return original path
	result, err := DecompressFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("DecompressFile failed: %v", err)
	}

	if result != tmpFile.Name() {
		t.Errorf("Expected original path, got different path")
	}
}

