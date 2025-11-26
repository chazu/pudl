package streaming

import (
	"fmt"
	"reflect"
	"sync"
)

// SimpleSchemaDetector implements basic pattern-based schema detection
type SimpleSchemaDetector struct {
	mu            sync.RWMutex
	patterns      []SchemaPattern
	samples       []ChunkSample
	maxSamples    int
	detectedSchema string
}

// SchemaPattern represents a pattern for detecting a specific schema
type SchemaPattern struct {
	Name        string            // Schema name (e.g., "aws.ec2-instance")
	Description string            // Human-readable description
	Fields      []FieldPattern    // Required field patterns
	Optional    []FieldPattern    // Optional field patterns
	Tags        map[string]string // Additional metadata
}

// FieldPattern represents a pattern for detecting specific fields
type FieldPattern struct {
	Name     string      // Field name or pattern
	Type     string      // Expected data type
	Required bool        // Whether field is required
	Pattern  string      // Regex pattern for field name (optional)
	Values   []string    // Expected values (for enums)
}

// ChunkSample represents a sample from a processed chunk for schema detection
type ChunkSample struct {
	Objects  []interface{}          // Parsed objects from chunk
	Format   string                 // Data format (json, csv, yaml)
	Metadata map[string]interface{} // Chunk metadata
}

// NewSimpleSchemaDetector creates a new simple schema detector
func NewSimpleSchemaDetector(maxSamples int) *SimpleSchemaDetector {
	detector := &SimpleSchemaDetector{
		patterns:   make([]SchemaPattern, 0),
		samples:    make([]ChunkSample, 0),
		maxSamples: maxSamples,
	}
	
	// Load default patterns
	detector.loadDefaultPatterns()
	
	return detector
}

// AddSample adds a chunk sample for schema detection
func (d *SimpleSchemaDetector) AddSample(chunk *ProcessedChunk) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	sample := ChunkSample{
		Objects:  chunk.Objects,
		Format:   chunk.Format,
		Metadata: chunk.Metadata,
	}
	
	d.samples = append(d.samples, sample)
	
	// Keep only the most recent samples
	if len(d.samples) > d.maxSamples {
		d.samples = d.samples[1:]
	}
	
	return nil
}

// DetectSchema returns the detected schema based on accumulated samples
func (d *SimpleSchemaDetector) DetectSchema() (*SchemaDetection, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	if len(d.samples) == 0 {
		return nil, fmt.Errorf("no samples available for schema detection")
	}
	
	// Analyze all objects from all samples
	allObjects := make([]interface{}, 0)
	for _, sample := range d.samples {
		allObjects = append(allObjects, sample.Objects...)
	}
	
	if len(allObjects) == 0 {
		return nil, fmt.Errorf("no objects found in samples")
	}
	
	// Find the best matching pattern
	bestMatch := d.findBestMatch(allObjects)
	
	if bestMatch == nil {
		return &SchemaDetection{
			SchemaName: "unknown",
			Confidence: 0.0,
			Samples:    len(d.samples),
			Metadata: map[string]interface{}{
				"total_objects": len(allObjects),
				"reason":        "no matching patterns found",
			},
		}, nil
	}
	
	return bestMatch, nil
}

// Reset clears all samples and starts fresh
func (d *SimpleSchemaDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.samples = d.samples[:0]
	d.detectedSchema = ""
}

// GetConfidence returns the current confidence level (simplified for now)
func (d *SimpleSchemaDetector) GetConfidence() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	if len(d.samples) == 0 {
		return 0.0
	}
	
	// Simple confidence based on number of samples
	confidence := float64(len(d.samples)) / float64(d.maxSamples)
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return confidence
}

// findBestMatch finds the best matching schema pattern for the given objects
func (d *SimpleSchemaDetector) findBestMatch(objects []interface{}) *SchemaDetection {
	bestScore := 0.0
	var bestPattern *SchemaPattern
	
	for _, pattern := range d.patterns {
		score := d.scorePattern(pattern, objects)
		if score > bestScore {
			bestScore = score
			bestPattern = &pattern
		}
	}
	
	if bestPattern == nil || bestScore < 0.3 { // Minimum threshold
		return nil
	}
	
	return &SchemaDetection{
		SchemaName: bestPattern.Name,
		Confidence: bestScore,
		Samples:    len(d.samples),
		Metadata: map[string]interface{}{
			"pattern_description": bestPattern.Description,
			"score":              bestScore,
			"total_objects":      len(objects),
		},
	}
}

// scorePattern calculates how well a pattern matches the given objects
func (d *SimpleSchemaDetector) scorePattern(pattern SchemaPattern, objects []interface{}) float64 {
	if len(objects) == 0 {
		return 0.0
	}
	
	totalScore := 0.0
	validObjects := 0
	
	for _, obj := range objects {
		objMap, ok := obj.(map[string]interface{})
		if !ok {
			continue
		}
		
		validObjects++
		score := d.scoreObject(pattern, objMap)
		totalScore += score
	}
	
	if validObjects == 0 {
		return 0.0
	}
	
	return totalScore / float64(validObjects)
}

// scoreObject calculates how well a pattern matches a single object
func (d *SimpleSchemaDetector) scoreObject(pattern SchemaPattern, obj map[string]interface{}) float64 {
	requiredMatches := 0
	optionalMatches := 0
	
	// Check required fields
	for _, field := range pattern.Fields {
		if d.matchesField(field, obj) {
			requiredMatches++
		}
	}
	
	// Check optional fields
	for _, field := range pattern.Optional {
		if d.matchesField(field, obj) {
			optionalMatches++
		}
	}
	
	// Calculate score
	requiredScore := 0.0
	if len(pattern.Fields) > 0 {
		requiredScore = float64(requiredMatches) / float64(len(pattern.Fields))
	}
	
	optionalScore := 0.0
	if len(pattern.Optional) > 0 {
		optionalScore = float64(optionalMatches) / float64(len(pattern.Optional))
	}
	
	// Weight required fields more heavily
	return (requiredScore * 0.8) + (optionalScore * 0.2)
}

// matchesField checks if an object field matches a field pattern
func (d *SimpleSchemaDetector) matchesField(field FieldPattern, obj map[string]interface{}) bool {
	value, exists := obj[field.Name]
	if !exists {
		return false
	}
	
	// Check type if specified
	if field.Type != "" {
		actualType := d.getValueType(value)
		if actualType != field.Type {
			return false
		}
	}
	
	// Check enum values if specified
	if len(field.Values) > 0 {
		valueStr := fmt.Sprintf("%v", value)
		found := false
		for _, expectedValue := range field.Values {
			if valueStr == expectedValue {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	return true
}

// getValueType returns a simplified type string for a value
func (d *SimpleSchemaDetector) getValueType(value interface{}) string {
	if value == nil {
		return "null"
	}
	
	switch reflect.TypeOf(value).Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map:
		return "object"
	default:
		return "unknown"
	}
}

// loadDefaultPatterns initializes the pattern list.
// Patterns should be loaded dynamically from CUE schemas using LoadPatternsFromCUE()
// or added manually using AddPattern().
func (d *SimpleSchemaDetector) loadDefaultPatterns() {
	// No default patterns - patterns should come from CUE schemas
	// Use CUESchemaDetector.LoadPatternsFromCUE() to populate patterns from schema repository
}

// AddPattern adds a custom schema pattern
func (d *SimpleSchemaDetector) AddPattern(pattern SchemaPattern) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.patterns = append(d.patterns, pattern)
}

// GetPatterns returns all registered patterns
func (d *SimpleSchemaDetector) GetPatterns() []SchemaPattern {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	patterns := make([]SchemaPattern, len(d.patterns))
	copy(patterns, d.patterns)
	return patterns
}
