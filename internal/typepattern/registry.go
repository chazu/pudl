package typepattern

import (
	"sort"
	"sync"
)

// Registry manages type patterns and provides detection capabilities.
type Registry struct {
	mu       sync.RWMutex
	patterns []*TypePattern
}

// NewRegistry creates a new Registry with no default patterns.
// Use Register to add patterns after creation.
func NewRegistry() *Registry {
	return &Registry{
		patterns: make([]*TypePattern, 0),
	}
}

// Register adds a pattern to the registry.
// Patterns are kept sorted by Priority (highest first).
func (r *Registry) Register(p *TypePattern) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.patterns = append(r.patterns, p)

	// Sort patterns by priority (highest first)
	sort.Slice(r.patterns, func(i, j int) bool {
		return r.patterns[i].Priority > r.patterns[j].Priority
	})
}

// Detect tries all patterns against the data and returns the best match.
// Returns nil if no pattern matches with sufficient confidence.
func (r *Registry) Detect(data map[string]interface{}) *DetectedType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var bestMatch *DetectedType

	for _, pattern := range r.patterns {
		confidence := r.calculateConfidence(pattern, data)
		if confidence <= 0 {
			continue
		}

		detected := &DetectedType{
			Pattern:    pattern,
			Confidence: confidence,
		}

		// Extract type ID if extractor is provided
		if pattern.TypeExtractor != nil {
			detected.TypeID = pattern.TypeExtractor(data)
		}

		// Map to import path if mapper is provided
		if pattern.ImportMapper != nil && detected.TypeID != "" {
			detected.ImportPath = pattern.ImportMapper(detected.TypeID)
		}

		// Extract definition name from type ID (part after the colon)
		if detected.TypeID != "" {
			detected.Definition = extractDefinition(detected.TypeID)
		}

		// Keep the best match (highest confidence)
		if bestMatch == nil || confidence > bestMatch.Confidence {
			bestMatch = detected
		}
	}

	return bestMatch
}

// GetPatternsByEcosystem returns all patterns belonging to the specified ecosystem.
func (r *Registry) GetPatternsByEcosystem(ecosystem string) []*TypePattern {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*TypePattern
	for _, p := range r.patterns {
		if p.Ecosystem == ecosystem {
			result = append(result, p)
		}
	}
	return result
}

// calculateConfidence computes how well a pattern matches the data.
// Returns 0 if required fields are missing, otherwise returns a score from 0.0 to 1.0.
func (r *Registry) calculateConfidence(pattern *TypePattern, data map[string]interface{}) float64 {
	// Check required fields - all must be present
	for _, field := range pattern.RequiredFields {
		if _, exists := data[field]; !exists {
			return 0
		}
	}

	// Base confidence from having all required fields
	confidence := 0.5

	// Boost for optional fields
	if len(pattern.OptionalFields) > 0 {
		optionalFound := 0
		for _, field := range pattern.OptionalFields {
			if _, exists := data[field]; exists {
				optionalFound++
			}
		}
		optionalBoost := float64(optionalFound) / float64(len(pattern.OptionalFields)) * 0.3
		confidence += optionalBoost
	}

	// Boost for matching field values
	if len(pattern.FieldValues) > 0 {
		valueMatches := 0
		totalChecks := len(pattern.FieldValues)
		for field, expectedValues := range pattern.FieldValues {
			if val, exists := data[field]; exists {
				if strVal, ok := val.(string); ok {
					for _, expected := range expectedValues {
						if strVal == expected {
							valueMatches++
							break
						}
					}
				}
			}
		}
		valueBoost := float64(valueMatches) / float64(totalChecks) * 0.2
		confidence += valueBoost
	}

	return confidence
}

// extractDefinition extracts the definition name from a type ID.
// For "batch/v1:Job", returns "Job". For "Job", returns "Job".
func extractDefinition(typeID string) string {
	for i := len(typeID) - 1; i >= 0; i-- {
		if typeID[i] == ':' {
			return typeID[i+1:]
		}
	}
	return typeID
}

