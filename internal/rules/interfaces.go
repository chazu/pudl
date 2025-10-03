package rules

import (
	"context"
	"time"
)

// RuleEngine defines the interface for pluggable schema assignment rule systems
type RuleEngine interface {
	// Initialize sets up the rule engine with configuration
	Initialize(config *Config) error

	// AssignSchema determines the best schema for given data
	AssignSchema(ctx context.Context, data interface{}, origin, format string) (*Result, error)

	// GetInfo returns information about the rule engine
	GetInfo() *EngineInfo

	// Close releases any resources held by the engine
	Close() error
}

// Result represents the output of schema assignment
type Result struct {
	// Core assignment result
	Schema     string  `json:"schema"`     // Assigned schema name (e.g., "aws.#EC2Instance")
	Confidence float64 `json:"confidence"` // Confidence score (0.0 to 1.0)

	// Additional metadata
	RuleName string            `json:"rule_name,omitempty"` // Name of the rule that matched
	Metadata map[string]interface{} `json:"metadata,omitempty"`  // Additional rule-specific data
	Duration time.Duration     `json:"duration,omitempty"`  // Time taken for assignment

	// Error information
	Warnings []string `json:"warnings,omitempty"` // Non-fatal warnings
}

// Config holds rule engine configuration
type Config struct {
	// Engine selection
	Type string `yaml:"type"` // "legacy", "zygomys"

	// Rule management
	RuleDir     string   `yaml:"rule_dir"`     // Directory containing rule files
	RuleFiles   []string `yaml:"rule_files"`   // Specific rule files to load
	EnabledRules []string `yaml:"enabled_rules"` // Specific rules to enable

	// Performance settings
	TimeoutMS   int  `yaml:"timeout_ms"`   // Execution timeout in milliseconds
	MaxMemoryMB int  `yaml:"max_memory_mb"` // Memory limit in MB
	CacheRules  bool `yaml:"cache_rules"`  // Whether to cache compiled rules

	// Debugging
	Debug   bool `yaml:"debug"`   // Enable debug logging
	Verbose bool `yaml:"verbose"` // Enable verbose output

	// Engine-specific configuration
	Custom map[string]interface{} `yaml:"custom"` // Engine-specific settings
}

// EngineInfo provides information about a rule engine
type EngineInfo struct {
	Name        string `json:"name"`        // Engine name (e.g., "Legacy", "Zygomys")
	Version     string `json:"version"`     // Engine version
	Description string `json:"description"` // Human-readable description
	RuleCount   int    `json:"rule_count"`  // Number of loaded rules
}

// DefaultConfig returns a default configuration for rule engines
func DefaultConfig() *Config {
	return &Config{
		Type:        "legacy", // Start with legacy for backward compatibility
		RuleDir:     "",       // Will be set by initialization
		TimeoutMS:   5000,     // 5 second timeout
		MaxMemoryMB: 50,       // 50MB memory limit
		CacheRules:  true,     // Enable rule caching
		Debug:       false,
		Verbose:     false,
		Custom:      make(map[string]interface{}),
	}
}

// Registry manages available rule engines
type Registry struct {
	engines map[string]func() RuleEngine
}

// NewRegistry creates a new rule engine registry
func NewRegistry() *Registry {
	return &Registry{
		engines: make(map[string]func() RuleEngine),
	}
}

// Register registers a rule engine factory function
func (r *Registry) Register(name string, factory func() RuleEngine) {
	r.engines[name] = factory
}

// Create creates a new rule engine instance by name
func (r *Registry) Create(name string) (RuleEngine, error) {
	factory, exists := r.engines[name]
	if !exists {
		return nil, &RuleEngineError{
			Code:    ErrorCodeUnknownEngine,
			Message: "unknown rule engine type: " + name,
		}
	}
	return factory(), nil
}

// List returns the names of all registered rule engines
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.engines))
	for name := range r.engines {
		names = append(names, name)
	}
	return names
}

// Global registry instance
var GlobalRegistry = NewRegistry()
