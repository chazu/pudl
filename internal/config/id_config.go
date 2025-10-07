package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pudl/internal/idgen"
)

// IDGenerationConfig holds configuration for ID generation
type IDGenerationConfig struct {
	// Default format to use for new IDs
	DefaultFormat idgen.IDFormat `json:"default_format"`
	
	// Whether to enable human-friendly IDs (vs legacy format)
	EnableFriendlyIDs bool `json:"enable_friendly_ids"`
	
	// Format overrides for specific origins
	OriginOverrides map[string]idgen.IDConfig `json:"origin_overrides,omitempty"`
	
	// Global prefix for all IDs (optional)
	GlobalPrefix string `json:"global_prefix,omitempty"`
	
	// Whether to maintain backward compatibility with legacy IDs
	LegacyCompatibility bool `json:"legacy_compatibility"`
}

// DefaultIDConfig returns sensible defaults for ID generation
func DefaultIDConfig() *IDGenerationConfig {
	return &IDGenerationConfig{
		DefaultFormat:       idgen.FormatProquint,
		EnableFriendlyIDs:   true,
		LegacyCompatibility: true,
		OriginOverrides: map[string]idgen.IDConfig{
			"aws":        idgen.DefaultConfigs["aws"],
			"kubernetes": idgen.DefaultConfigs["kubernetes"],
			"collection": idgen.DefaultConfigs["collections"],
		},
	}
}

// GetConfigForOrigin returns the appropriate ID config for a given origin
func (c *IDGenerationConfig) GetConfigForOrigin(origin string) idgen.IDConfig {
	// Check for exact match first
	if config, exists := c.OriginOverrides[origin]; exists {
		return config
	}
	
	// Check for partial matches
	for pattern, config := range c.OriginOverrides {
		if containsIgnoreCase(origin, pattern) {
			return config
		}
	}
	
	// Return default config
	return idgen.IDConfig{
		Format: c.DefaultFormat,
		Prefix: c.GlobalPrefix,
	}
}

// ShouldUseFriendlyIDs returns whether to use friendly IDs for new entries
func (c *IDGenerationConfig) ShouldUseFriendlyIDs() bool {
	return c.EnableFriendlyIDs
}

// ShouldMaintainLegacyCompatibility returns whether to maintain legacy ID support
func (c *IDGenerationConfig) ShouldMaintainLegacyCompatibility() bool {
	return c.LegacyCompatibility
}

// ConfigManager handles loading and saving ID configuration
type ConfigManager struct {
	configPath string
	config     *IDGenerationConfig
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configDir string) *ConfigManager {
	configPath := filepath.Join(configDir, "id_config.json")
	return &ConfigManager{
		configPath: configPath,
		config:     nil,
	}
}

// LoadConfig loads configuration from file, creating defaults if not found
func (m *ConfigManager) LoadConfig() (*IDGenerationConfig, error) {
	if m.config != nil {
		return m.config, nil
	}
	
	// Check if config file exists
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		// Create default config
		m.config = DefaultIDConfig()
		if err := m.SaveConfig(); err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
		return m.config, nil
	}
	
	// Load existing config
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	var config IDGenerationConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	m.config = &config
	return m.config, nil
}

// SaveConfig saves the current configuration to file
func (m *ConfigManager) SaveConfig() error {
	if m.config == nil {
		return fmt.Errorf("no config to save")
	}
	
	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(m.configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	// Marshal config to JSON
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	// Write to file
	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	return nil
}

// UpdateConfig updates the configuration
func (m *ConfigManager) UpdateConfig(config *IDGenerationConfig) error {
	m.config = config
	return m.SaveConfig()
}

// GetConfig returns the current configuration
func (m *ConfigManager) GetConfig() *IDGenerationConfig {
	if m.config == nil {
		// Return defaults if not loaded
		return DefaultIDConfig()
	}
	return m.config
}

// SetDefaultFormat updates the default ID format
func (m *ConfigManager) SetDefaultFormat(format idgen.IDFormat) error {
	config := m.GetConfig()
	config.DefaultFormat = format
	return m.UpdateConfig(config)
}

// SetOriginOverride sets a format override for a specific origin pattern
func (m *ConfigManager) SetOriginOverride(pattern string, config idgen.IDConfig) error {
	cfg := m.GetConfig()
	if cfg.OriginOverrides == nil {
		cfg.OriginOverrides = make(map[string]idgen.IDConfig)
	}
	cfg.OriginOverrides[pattern] = config
	return m.UpdateConfig(cfg)
}

// EnableFriendlyIDs enables or disables friendly ID generation
func (m *ConfigManager) EnableFriendlyIDs(enable bool) error {
	config := m.GetConfig()
	config.EnableFriendlyIDs = enable
	return m.UpdateConfig(config)
}

// SetLegacyCompatibility enables or disables legacy ID compatibility
func (m *ConfigManager) SetLegacyCompatibility(enable bool) error {
	config := m.GetConfig()
	config.LegacyCompatibility = enable
	return m.UpdateConfig(config)
}

// ValidateConfig validates the configuration for consistency
func (c *IDGenerationConfig) ValidateConfig() error {
	// Validate default format
	validFormats := []idgen.IDFormat{
		idgen.FormatShortCode,
		idgen.FormatReadable,
		idgen.FormatCompact,
		idgen.FormatSequential,
		idgen.FormatLegacy,
	}
	
	valid := false
	for _, format := range validFormats {
		if c.DefaultFormat == format {
			valid = true
			break
		}
	}
	
	if !valid {
		return fmt.Errorf("invalid default format: %s", c.DefaultFormat)
	}
	
	// Validate origin overrides
	for origin, config := range c.OriginOverrides {
		valid = false
		for _, format := range validFormats {
			if config.Format == format {
				valid = true
				break
			}
		}
		
		if !valid {
			return fmt.Errorf("invalid format for origin %s: %s", origin, config.Format)
		}
	}
	
	return nil
}

// Helper function for case-insensitive substring matching
func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}
