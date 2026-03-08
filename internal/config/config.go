package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"pudl/internal/errors"
)

type Config struct {
	SchemaPath   string `yaml:"schema_path"`
	DataPath     string `yaml:"data_path"`
	Version      string `yaml:"version"`
	VaultBackend string `yaml:"vault_backend,omitempty"`
}

func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	pudlDir := filepath.Join(homeDir, ".pudl")

	return &Config{
		SchemaPath: filepath.Join(pudlDir, "schema"),
		DataPath:   filepath.Join(pudlDir, "data"),
		Version:    "1.0",
	}
}

func GetPudlDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home dir is not available
		return ".pudl"
	}
	return filepath.Join(homeDir, ".pudl")
}

func GetConfigPath() string {
	return filepath.Join(GetPudlDir(), "config.yaml")
}

func Load() (*Config, error) {
	configPath := GetConfigPath()

	// If config doesn't exist, return default config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeFileSystem, "Failed to read config file", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.NewConfigError("Failed to parse config file - invalid YAML format", err)
	}

	return &config, nil
}

func (c *Config) Save() error {
	configPath := GetConfigPath()

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to create config directory", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return errors.WrapError(errors.ErrCodeConfigInvalid, "Failed to marshal config", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to write config file", err)
	}

	return nil
}

// Exists checks if the PUDL workspace exists
func Exists() bool {
	pudlDir := GetPudlDir()
	configPath := GetConfigPath()

	// Check if both the directory and config file exist
	if _, err := os.Stat(pudlDir); os.IsNotExist(err) {
		return false
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return false
	}

	return true
}

func ValidConfigKeys() []string {
	return []string{"schema_path", "data_path", "version", "vault_backend"}
}

func IsValidConfigKey(key string) bool {
	validKeys := ValidConfigKeys()
	for _, validKey := range validKeys {
		if key == validKey {
			return true
		}
	}
	return false
}

func ValidatePath(path string) error {
	// Expand ~ to home directory if present
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot expand home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check if path exists
	if _, err := os.Stat(absPath); err == nil {
		// Path exists, check if it's a directory
		if info, err := os.Stat(absPath); err == nil && !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", absPath)
		}
		return nil
	}

	// Path doesn't exist, check if parent directory exists and is writable
	parentDir := filepath.Dir(absPath)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		return fmt.Errorf("parent directory does not exist: %s", parentDir)
	}

	// Check if we can write to parent directory by attempting to create a temp file
	tempFile := filepath.Join(parentDir, ".pudl_temp_test")
	if err := os.WriteFile(tempFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("cannot write to parent directory: %s", parentDir)
	}
	os.Remove(tempFile) // Clean up

	return nil
}

func SetConfigValue(key, value string) error {
	if !IsValidConfigKey(key) {
		return errors.NewInputError(
			fmt.Sprintf("Invalid configuration key: %s", key),
			fmt.Sprintf("Valid keys are: %s", strings.Join(ValidConfigKeys(), ", ")),
			"Use 'pudl config' to see current configuration")
	}

	// Load current configuration
	cfg, err := Load()
	if err != nil {
		return err // Already a PUDLError from Load()
	}

	// Validate and set the value based on key
	switch key {
	case "schema_path":
		if err := ValidatePath(value); err != nil {
			return errors.NewInputError(
				fmt.Sprintf("Invalid schema_path: %s", value),
				"Ensure the path is valid and accessible",
				"Use an absolute path or ensure parent directories exist")
		}
		// Expand ~ to absolute path
		if strings.HasPrefix(value, "~/") {
			homeDir, _ := os.UserHomeDir()
			value = filepath.Join(homeDir, value[2:])
		}
		absPath, _ := filepath.Abs(value)
		cfg.SchemaPath = absPath

	case "data_path":
		if err := ValidatePath(value); err != nil {
			return errors.NewInputError(
				fmt.Sprintf("Invalid data_path: %s", value),
				"Ensure the path is valid and accessible",
				"Use an absolute path or ensure parent directories exist")
		}
		// Expand ~ to absolute path
		if strings.HasPrefix(value, "~/") {
			homeDir, _ := os.UserHomeDir()
			value = filepath.Join(homeDir, value[2:])
		}
		absPath, _ := filepath.Abs(value)
		cfg.DataPath = absPath

	case "version":
		if strings.TrimSpace(value) == "" {
			return errors.NewInputError("Version cannot be empty", "Provide a valid version string")
		}
		cfg.Version = value

	case "vault_backend":
		if value != "env" && value != "file" && value != "" {
			return errors.NewInputError(
				fmt.Sprintf("Invalid vault_backend: %s", value),
				"Valid values are: env, file")
		}
		cfg.VaultBackend = value
	}

	// Save the updated configuration
	if err := cfg.Save(); err != nil {
		return err // Already a PUDLError from Save()
	}

	return nil
}

func ResetToDefaults() error {
	defaultCfg := DefaultConfig()
	if err := defaultCfg.Save(); err != nil {
		return err // Already a PUDLError from Save()
	}
	return nil
}
