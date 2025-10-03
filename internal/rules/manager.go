package rules

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

// Manager handles rule engine lifecycle and provides a unified API
type Manager struct {
	mu           sync.RWMutex
	engine       RuleEngine
	configMgr    *ConfigManager
	initialized  bool
	pudlHome     string
}

// NewManager creates a new rule engine manager
func NewManager(pudlHome string) *Manager {
	configPath := filepath.Join(pudlHome, "rules", "config.yaml")
	return &Manager{
		configMgr: NewConfigManager(configPath),
		pudlHome:  pudlHome,
	}
}

// Initialize sets up the rule engine manager
func (m *Manager) Initialize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load configuration
	if err := m.configMgr.Load(); err != nil {
		return fmt.Errorf("failed to load rule engine configuration: %w", err)
	}

	config := m.configMgr.Get()

	// Set default rule directory if not specified
	if config.RuleDir == "" {
		config.RuleDir = GetDefaultRuleDir(m.pudlHome)
		m.configMgr.Set(config)
	}

	// Create rule engine
	engine, err := GlobalRegistry.Create(config.Type)
	if err != nil {
		return fmt.Errorf("failed to create rule engine: %w", err)
	}

	// Initialize the engine
	if err := engine.Initialize(config); err != nil {
		return fmt.Errorf("failed to initialize rule engine: %w", err)
	}

	m.engine = engine
	m.initialized = true

	return nil
}

// AssignSchema assigns a schema to data using the configured rule engine
func (m *Manager) AssignSchema(data interface{}, origin, format string) (string, float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.initialized {
		return "", 0, fmt.Errorf("rule engine manager not initialized")
	}

	// Create context with timeout
	config := m.configMgr.Get()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.TimeoutMS)*time.Millisecond)
	defer cancel()

	// Call the rule engine
	result, err := m.engine.AssignSchema(ctx, data, origin, format)
	if err != nil {
		return "", 0, fmt.Errorf("rule engine assignment failed: %w", err)
	}

	return result.Schema, result.Confidence, nil
}

// AssignSchemaDetailed returns detailed schema assignment results
func (m *Manager) AssignSchemaDetailed(data interface{}, origin, format string) (*Result, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.initialized {
		return nil, fmt.Errorf("rule engine manager not initialized")
	}

	// Create context with timeout
	config := m.configMgr.Get()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.TimeoutMS)*time.Millisecond)
	defer cancel()

	// Call the rule engine
	result, err := m.engine.AssignSchema(ctx, data, origin, format)
	if err != nil {
		return nil, fmt.Errorf("rule engine assignment failed: %w", err)
	}

	return result, nil
}

// GetEngineInfo returns information about the current rule engine
func (m *Manager) GetEngineInfo() (*EngineInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.initialized {
		return nil, fmt.Errorf("rule engine manager not initialized")
	}

	return m.engine.GetInfo(), nil
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.configMgr.Get()
}

// UpdateConfig updates the rule engine configuration
func (m *Manager) UpdateConfig(updates map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update configuration
	if err := m.configMgr.Update(updates); err != nil {
		return fmt.Errorf("failed to update configuration: %w", err)
	}

	// Save configuration
	if err := m.configMgr.Save(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	// If engine type changed, reinitialize
	config := m.configMgr.Get()
	if m.initialized && m.engine != nil {
		currentInfo := m.engine.GetInfo()
		if currentInfo.Name != config.Type {
			return m.reinitialize()
		}
	}

	return nil
}

// SwitchEngine switches to a different rule engine type
func (m *Manager) SwitchEngine(engineType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update configuration
	config := m.configMgr.Get()
	config.Type = engineType
	if err := m.configMgr.Set(config); err != nil {
		return fmt.Errorf("failed to set new engine type: %w", err)
	}

	// Save configuration
	if err := m.configMgr.Save(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	// Reinitialize with new engine
	return m.reinitialize()
}

// reinitialize recreates the rule engine (must be called with lock held)
func (m *Manager) reinitialize() error {
	// Close existing engine
	if m.engine != nil {
		m.engine.Close()
	}

	// Create new engine
	config := m.configMgr.Get()
	engine, err := GlobalRegistry.Create(config.Type)
	if err != nil {
		m.initialized = false
		return fmt.Errorf("failed to create new rule engine: %w", err)
	}

	// Initialize the new engine
	if err := engine.Initialize(config); err != nil {
		m.initialized = false
		return fmt.Errorf("failed to initialize new rule engine: %w", err)
	}

	m.engine = engine
	m.initialized = true

	return nil
}

// Close shuts down the rule engine manager
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.engine != nil {
		if err := m.engine.Close(); err != nil {
			return fmt.Errorf("failed to close rule engine: %w", err)
		}
	}

	m.engine = nil
	m.initialized = false

	return nil
}

// IsInitialized returns whether the manager is initialized
func (m *Manager) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.initialized
}

// ListAvailableEngines returns the names of all available rule engines
func (m *Manager) ListAvailableEngines() []string {
	return GlobalRegistry.List()
}
