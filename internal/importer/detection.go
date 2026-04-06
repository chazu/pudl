package importer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// detectFormat detects the format of a file based on extension and content
func (i *Importer) detectFormat(filePath string) (string, error) {
	// First detect and handle compression
	compression := DetectCompression(filePath)
	var fileToAnalyze string
	var err error

	if compression != "none" {
		// File is compressed, decompress it first
		fileToAnalyze, err = DecompressFile(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to decompress file: %w", err)
		}
		// Clean up temporary decompressed file after detection
		defer os.Remove(fileToAnalyze)
	} else {
		fileToAnalyze = filePath
	}

	ext := strings.ToLower(filepath.Ext(fileToAnalyze))

	// First try extension-based detection
	switch ext {
	case ".ndjson", ".jsonl":
		return "ndjson", nil
	case ".json":
		// Check if it's NDJSON (newline-delimited JSON)
		if isNDJSON, err := i.isNewlineDelimitedJSON(fileToAnalyze); err == nil && isNDJSON {
			return "ndjson", nil
		}
		return "json", nil
	case ".yaml", ".yml":
		return "yaml", nil
	case ".csv":
		return "csv", nil
	}

	// If extension is unclear, try content-based detection
	file, err := os.Open(fileToAnalyze)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read first 4KB for content detection
	buffer := make([]byte, 4096)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	content := string(buffer[:n])
	content = strings.TrimSpace(content)

	// Try to detect JSON-like content
	if (strings.HasPrefix(content, "{") && strings.Contains(content, "}")) ||
		(strings.HasPrefix(content, "[") && strings.Contains(content, "]")) {
		// Content looks like JSON — but check for NDJSON first.
		// NDJSON is multiple JSON objects separated by newlines, not a JSON array.
		if strings.HasPrefix(content, "{") {
			if isNDJSON, err := i.isNewlineDelimitedJSON(fileToAnalyze); err == nil && isNDJSON {
				return "ndjson", nil
			}
		}
		return "json", nil
	}

	// Try to detect YAML
	if strings.Contains(content, ":") && !strings.Contains(content, ",") {
		return "yaml", nil
	}

	// Try to detect CSV
	if strings.Contains(content, ",") && strings.Contains(content, "\n") {
		return "csv", nil
	}

	// Default to unknown
	return "unknown", nil
}

// isNewlineDelimitedJSON checks if a file contains newline-delimited JSON
func (i *Importer) isNewlineDelimitedJSON(filePath string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// Read first few KB to check format
	buffer := make([]byte, 4096)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false, err
	}

	content := string(buffer[:n])
	lines := strings.Split(content, "\n")

	// Need at least 2 lines for NDJSON
	if len(lines) < 2 {
		return false, nil
	}

	jsonLines := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Check if line looks like JSON object
		if (strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}")) ||
			(strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]")) {
			// Try to parse as JSON to confirm
			var obj interface{}
			if json.Unmarshal([]byte(line), &obj) == nil {
				jsonLines++
			}
		}
	}

	// Consider it NDJSON if we have multiple valid JSON lines
	return jsonLines >= 2, nil
}

// detectOrigin returns the origin/source identifier for the data.
// It uses the filename (without extension) as the origin identifier.
// Schema detection should be handled by CUE-based inference, not hardcoded patterns.
func (i *Importer) detectOrigin(filePath, format string) string {
	filename := strings.ToLower(filepath.Base(filePath))

	// Remove extension to get the base name
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	if name != "" {
		return name
	}

	return "unknown-source"
}
