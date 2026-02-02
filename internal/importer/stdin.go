package importer

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadStdinToTempFile reads data from stdin and writes it to a temporary file
// Returns the path to the temporary file
func ReadStdinToTempFile() (string, error) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "pudl-stdin-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer tmpFile.Close()

	// Copy stdin to temporary file
	if _, err := io.Copy(tmpFile, os.Stdin); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to read from stdin: %w", err)
	}

	return tmpFile.Name(), nil
}

// DetectFormatFromContent detects the format of data from file content
// This is used when format cannot be determined from filename
func DetectFormatFromContent(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read first 1KB for content detection
	buffer := make([]byte, 1024)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if n == 0 {
		return "unknown", nil
	}

	content := string(buffer[:n])
	content = strings.TrimSpace(content)

	// Try to detect JSON
	if (strings.HasPrefix(content, "{") && strings.Contains(content, "}")) ||
		(strings.HasPrefix(content, "[") && strings.Contains(content, "]")) {
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

// IsStdinAvailable checks if there is data available on stdin
// Returns true if stdin has data or is being piped
func IsStdinAvailable() bool {
	// Check if stdin is a pipe or has data
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	// Check if stdin is a pipe (mode has CharDevice bit unset)
	// If it's a pipe, data is available
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// GetStdinFilename generates a filename for stdin data
// Uses the provided format or detects it from content
func GetStdinFilename(format string) string {
	if format == "" || format == "unknown" {
		format = "json" // Default to JSON for stdin
	}

	// Create a filename based on format
	ext := "." + format
	if format == "ndjson" {
		ext = ".json" // NDJSON files have .json extension
	}

	return "stdin" + ext
}

