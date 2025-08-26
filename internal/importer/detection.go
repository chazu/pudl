package importer

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// detectFormat detects the format of a file based on extension and content
func (i *Importer) detectFormat(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	// First try extension-based detection
	switch ext {
	case ".json":
		return "json", nil
	case ".yaml", ".yml":
		return "yaml", nil
	case ".csv":
		return "csv", nil
	}

	// If extension is unclear, try content-based detection
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read first 1KB for content detection
	buffer := make([]byte, 1024)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
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

// detectOrigin attempts to detect the origin/source of the data
func (i *Importer) detectOrigin(filePath, format string) string {
	filename := strings.ToLower(filepath.Base(filePath))
	
	// Remove extension for analysis
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	
	// AWS patterns
	if strings.Contains(name, "aws") || strings.Contains(name, "ec2") || 
		strings.Contains(name, "s3") || strings.Contains(name, "rds") {
		if strings.Contains(name, "ec2") && strings.Contains(name, "instance") {
			return "aws-ec2-describe-instances"
		}
		if strings.Contains(name, "s3") && strings.Contains(name, "bucket") {
			return "aws-s3-list-buckets"
		}
		return "aws-unknown"
	}

	// Kubernetes patterns
	if strings.Contains(name, "k8s") || strings.Contains(name, "kube") || 
		strings.Contains(name, "pod") || strings.Contains(name, "service") {
		if strings.Contains(name, "pod") {
			return "k8s-get-pods"
		}
		if strings.Contains(name, "service") {
			return "k8s-get-services"
		}
		return "k8s-unknown"
	}

	// Generic patterns based on common terms
	if strings.Contains(name, "instance") {
		return "instances"
	}
	if strings.Contains(name, "server") {
		return "servers"
	}
	if strings.Contains(name, "metric") {
		return "metrics"
	}
	if strings.Contains(name, "log") {
		return "logs"
	}

	// If no pattern matches, use filename without extension
	if name != "" {
		return name
	}

	return "unknown-source"
}

// analyzeData reads and analyzes data to extract basic information
func (i *Importer) analyzeData(filePath, format string) (interface{}, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	switch format {
	case "json":
		return i.analyzeJSON(file)
	case "yaml":
		return i.analyzeYAML(file)
	case "csv":
		return i.analyzeCSV(file)
	default:
		// For unknown formats, just return basic info
		return map[string]interface{}{"format": "unknown"}, 1, nil
	}
}

// analyzeJSON analyzes JSON data
func (i *Importer) analyzeJSON(reader io.Reader) (interface{}, int, error) {
	var data interface{}
	decoder := json.NewDecoder(reader)
	
	if err := decoder.Decode(&data); err != nil {
		return nil, 0, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Count records
	recordCount := 1
	if arr, ok := data.([]interface{}); ok {
		recordCount = len(arr)
	}

	return data, recordCount, nil
}

// analyzeYAML analyzes YAML data
func (i *Importer) analyzeYAML(reader io.Reader) (interface{}, int, error) {
	var data interface{}
	decoder := yaml.NewDecoder(reader)
	
	if err := decoder.Decode(&data); err != nil {
		return nil, 0, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Count records
	recordCount := 1
	if arr, ok := data.([]interface{}); ok {
		recordCount = len(arr)
	}

	return data, recordCount, nil
}

// analyzeCSV analyzes CSV data
func (i *Importer) analyzeCSV(reader io.Reader) (interface{}, int, error) {
	csvReader := csv.NewReader(reader)
	
	// Read all records
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse CSV: %w", err)
	}

	if len(records) == 0 {
		return [][]string{}, 0, nil
	}

	// Convert to structured format with headers
	if len(records) > 1 {
		headers := records[0]
		var data []map[string]string
		
		for _, record := range records[1:] {
			row := make(map[string]string)
			for i, value := range record {
				if i < len(headers) {
					row[headers[i]] = value
				}
			}
			data = append(data, row)
		}
		
		return data, len(data), nil
	}

	return records, len(records), nil
}
