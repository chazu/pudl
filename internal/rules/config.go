package rules

import (
	"os"
	"path/filepath"
	"gopkg.in/yaml.v3"
)

// ConfigManager handles rule engine configuration
type ConfigManager struct {
	configPath string
	config     *Config
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath: configPath,
		config:     DefaultConfig(),
	}
}

// Load loads configuration from file, falling back to defaults if file doesn't exist
func (cm *ConfigManager) Load() error {
	// If config file doesn't exist, use defaults
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		cm.config = DefaultConfig()
		return nil
	}

	// Read config file
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return ErrConfigurationError("failed to read config file: " + err.Error())
	}

	// Parse YAML
	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return ErrConfigurationError("failed to parse config file: " + err.Error())
	}

	// Validate configuration
	if err := cm.validateConfig(config); err != nil {
		return err
	}

	cm.config = config
	return nil
}

// Save saves the current configuration to file
func (cm *ConfigManager) Save() error {
	// Ensure directory exists
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ErrConfigurationError("failed to create config directory: " + err.Error())
	}

	// Marshal to YAML
	data, err := yaml.Marshal(cm.config)
	if err != nil {
		return ErrConfigurationError("failed to marshal config: " + err.Error())
	}

	// Write to file
	if err := os.WriteFile(cm.configPath, data, 0644); err != nil {
		return ErrConfigurationError("failed to write config file: " + err.Error())
	}

	return nil
}

// Get returns the current configuration
func (cm *ConfigManager) Get() *Config {
	return cm.config
}

// Set updates the configuration
func (cm *ConfigManager) Set(config *Config) error {
	if err := cm.validateConfig(config); err != nil {
		return err
	}
	cm.config = config
	return nil
}

// Update updates specific configuration values
func (cm *ConfigManager) Update(updates map[string]interface{}) error {
	// Create a copy of current config
	newConfig := *cm.config

	// Apply updates
	for key, value := range updates {
		switch key {
		case "type":
			if str, ok := value.(string); ok {
				newConfig.Type = str
			} else {
				return ErrConfigurationError("type must be a string")
			}
		case "rule_dir":
			if str, ok := value.(string); ok {
				newConfig.RuleDir = str
			} else {
				return ErrConfigurationError("rule_dir must be a string")
			}
		case "timeout_ms":
			if num, ok := value.(int); ok {
				newConfig.TimeoutMS = num
			} else {
				return ErrConfigurationError("timeout_ms must be an integer")
			}
		case "max_memory_mb":
			if num, ok := value.(int); ok {
				newConfig.MaxMemoryMB = num
			} else {
				return ErrConfigurationError("max_memory_mb must be an integer")
			}
		case "debug":
			if b, ok := value.(bool); ok {
				newConfig.Debug = b
			} else {
				return ErrConfigurationError("debug must be a boolean")
			}
		case "verbose":
			if b, ok := value.(bool); ok {
				newConfig.Verbose = b
			} else {
				return ErrConfigurationError("verbose must be a boolean")
			}
		default:
			return ErrConfigurationError("unknown configuration key: " + key)
		}
	}

	// Validate and set
	if err := cm.validateConfig(&newConfig); err != nil {
		return err
	}

	cm.config = &newConfig
	return nil
}

// validateConfig validates a configuration
func (cm *ConfigManager) validateConfig(config *Config) error {
	// Validate engine type
	validTypes := []string{"legacy", "zygomys"}
	validType := false
	for _, t := range validTypes {
		if config.Type == t {
			validType = true
			break
		}
	}
	if !validType {
		return ErrConfigurationError("invalid engine type: " + config.Type)
	}

	// Validate timeout
	if config.TimeoutMS <= 0 {
		return ErrConfigurationError("timeout_ms must be positive")
	}

	// Validate memory limit
	if config.MaxMemoryMB <= 0 {
		return ErrConfigurationError("max_memory_mb must be positive")
	}

	// Validate rule directory if specified
	if config.RuleDir != "" {
		if info, err := os.Stat(config.RuleDir); err != nil {
			if !os.IsNotExist(err) {
				return ErrConfigurationError("rule_dir is not accessible: " + err.Error())
			}
			// Directory doesn't exist - that's okay, we'll create it
		} else if !info.IsDir() {
			return ErrConfigurationError("rule_dir is not a directory")
		}
	}

	return nil
}

// GetDefaultRuleDir returns the default rule directory path
func GetDefaultRuleDir(pudlHome string) string {
	return filepath.Join(pudlHome, "rules")
}

// InitializeDefaultConfig creates a default configuration with proper paths
func InitializeDefaultConfig(pudlHome string) *Config {
	config := DefaultConfig()
	config.RuleDir = GetDefaultRuleDir(pudlHome)
	return config
}
