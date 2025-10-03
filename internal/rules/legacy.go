package rules

import (
	"context"
	"strings"
	"time"
)

// LegacyRuleEngine wraps the existing hard-coded schema assignment logic
// This provides backward compatibility while enabling the new RuleEngine interface
type LegacyRuleEngine struct {
	config *Config
	info   *EngineInfo
}

// NewLegacyRuleEngine creates a new legacy rule engine
func NewLegacyRuleEngine() RuleEngine {
	return &LegacyRuleEngine{
		info: &EngineInfo{
			Name:        "Legacy",
			Version:     "1.0.0",
			Description: "Hard-coded rule engine with AWS, K8s, and generic detection rules",
			RuleCount:   7, // Number of hard-coded rule patterns
		},
	}
}

// Initialize sets up the legacy rule engine
func (e *LegacyRuleEngine) Initialize(config *Config) error {
	if config == nil {
		return ErrConfigurationError("config cannot be nil")
	}
	e.config = config
	return nil
}

// AssignSchema determines the best schema for given data using legacy rules
func (e *LegacyRuleEngine) AssignSchema(ctx context.Context, data interface{}, origin, format string) (*Result, error) {
	start := time.Now()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ErrExecutionTimeout(e.config.TimeoutMS)
	default:
	}

	// Convert data to map for analysis if possible
	var dataMap map[string]interface{}

	switch d := data.(type) {
	case map[string]interface{}:
		dataMap = d
	case []interface{}:
		// If it's an array, try to get the first element
		if len(d) > 0 {
			if firstItem, itemOk := d[0].(map[string]interface{}); itemOk {
				dataMap = firstItem
			}
		}
	}

	// If we couldn't extract a map, use catchall
	if dataMap == nil {
		return &Result{
			Schema:     "unknown.#CatchAll",
			Confidence: 0.1,
			RuleName:   "catchall-no-map",
			Duration:   time.Since(start),
			Warnings:   []string{"Could not extract map from data, using catchall schema"},
		}, nil
	}

	// Apply legacy detection rules in order of specificity

	// AWS EC2 Instance detection
	if e.hasFields(dataMap, []string{"InstanceId", "State", "InstanceType"}) {
		confidence := 0.9
		ruleName := "aws-ec2-instance"
		
		if instanceId, exists := dataMap["InstanceId"].(string); exists {
			if strings.HasPrefix(instanceId, "i-") && len(instanceId) >= 10 {
				confidence = 0.95
				ruleName = "aws-ec2-instance-validated"
			}
		}
		
		return &Result{
			Schema:     "aws.#EC2Instance",
			Confidence: confidence,
			RuleName:   ruleName,
			Duration:   time.Since(start),
			Metadata: map[string]interface{}{
				"detected_fields": []string{"InstanceId", "State", "InstanceType"},
			},
		}, nil
	}

	// AWS S3 Bucket detection
	if e.hasFields(dataMap, []string{"Name", "CreationDate"}) && 
		strings.Contains(strings.ToLower(origin), "s3") {
		return &Result{
			Schema:     "aws.#S3Bucket",
			Confidence: 0.9,
			RuleName:   "aws-s3-bucket",
			Duration:   time.Since(start),
			Metadata: map[string]interface{}{
				"detected_fields": []string{"Name", "CreationDate"},
				"origin_match":    "s3",
			},
		}, nil
	}

	// Kubernetes Pod detection
	if e.hasFields(dataMap, []string{"kind", "apiVersion", "metadata"}) {
		if kind, exists := dataMap["kind"].(string); exists && kind == "Pod" {
			return &Result{
				Schema:     "k8s.#Pod",
				Confidence: 0.95,
				RuleName:   "k8s-pod-exact",
				Duration:   time.Since(start),
				Metadata: map[string]interface{}{
					"detected_fields": []string{"kind", "apiVersion", "metadata"},
					"kind":           kind,
				},
			}, nil
		}
		
		if kind, exists := dataMap["kind"].(string); exists {
			return &Result{
				Schema:     "k8s.#" + kind,
				Confidence: 0.9,
				RuleName:   "k8s-resource-by-kind",
				Duration:   time.Since(start),
				Metadata: map[string]interface{}{
					"detected_fields": []string{"kind", "apiVersion", "metadata"},
					"kind":           kind,
				},
			}, nil
		}
		
		return &Result{
			Schema:     "k8s.#Resource",
			Confidence: 0.8,
			RuleName:   "k8s-resource-generic",
			Duration:   time.Since(start),
			Metadata: map[string]interface{}{
				"detected_fields": []string{"kind", "apiVersion", "metadata"},
			},
		}, nil
	}

	// AWS API Response pattern
	if e.hasFields(dataMap, []string{"ResponseMetadata"}) {
		return &Result{
			Schema:     "aws.#APIResponse",
			Confidence: 0.8,
			RuleName:   "aws-api-response",
			Duration:   time.Since(start),
			Metadata: map[string]interface{}{
				"detected_fields": []string{"ResponseMetadata"},
			},
		}, nil
	}

	// Origin-based fallback detection
	originLower := strings.ToLower(origin)
	
	if strings.Contains(originLower, "aws") {
		if strings.Contains(originLower, "ec2") {
			return &Result{
				Schema:     "aws.#EC2Resource",
				Confidence: 0.6,
				RuleName:   "aws-ec2-origin-fallback",
				Duration:   time.Since(start),
				Metadata: map[string]interface{}{
					"origin_match": "aws+ec2",
				},
			}, nil
		}
		
		if strings.Contains(originLower, "s3") {
			return &Result{
				Schema:     "aws.#S3Resource",
				Confidence: 0.6,
				RuleName:   "aws-s3-origin-fallback",
				Duration:   time.Since(start),
				Metadata: map[string]interface{}{
					"origin_match": "aws+s3",
				},
			}, nil
		}
		
		return &Result{
			Schema:     "aws.#Resource",
			Confidence: 0.5,
			RuleName:   "aws-origin-fallback",
			Duration:   time.Since(start),
			Metadata: map[string]interface{}{
				"origin_match": "aws",
			},
		}, nil
	}

	if strings.Contains(originLower, "k8s") || strings.Contains(originLower, "kube") {
		return &Result{
			Schema:     "k8s.#Resource",
			Confidence: 0.5,
			RuleName:   "k8s-origin-fallback",
			Duration:   time.Since(start),
			Metadata: map[string]interface{}{
				"origin_match": "k8s/kube",
			},
		}, nil
	}

	// Default to catchall
	return &Result{
		Schema:     "unknown.#CatchAll",
		Confidence: 0.1,
		RuleName:   "catchall-default",
		Duration:   time.Since(start),
		Warnings:   []string{"No specific rules matched, using catchall schema"},
	}, nil
}

// GetInfo returns information about the legacy rule engine
func (e *LegacyRuleEngine) GetInfo() *EngineInfo {
	return e.info
}

// Close releases any resources held by the engine (none for legacy)
func (e *LegacyRuleEngine) Close() error {
	return nil
}

// hasFields checks if a map contains all specified fields
func (e *LegacyRuleEngine) hasFields(data map[string]interface{}, fields []string) bool {
	for _, field := range fields {
		if _, exists := data[field]; !exists {
			return false
		}
	}
	return true
}

// Register the legacy rule engine with the global registry
func init() {
	GlobalRegistry.Register("legacy", NewLegacyRuleEngine)
}
