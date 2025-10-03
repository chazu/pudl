package rules

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRuleEngineManager(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pudl-rules-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create manager
	manager := NewManager(tempDir)

	// Test initialization
	if err := manager.Initialize(); err != nil {
		t.Fatalf("Failed to initialize manager: %v", err)
	}
	defer manager.Close()

	// Test that manager is initialized
	if !manager.IsInitialized() {
		t.Error("Manager should be initialized")
	}

	// Test basic schema assignment with legacy engine
	schema, confidence, err := manager.AssignSchema(
		map[string]interface{}{
			"InstanceId":   "i-1234567890abcdef0",
			"State":       map[string]interface{}{"Name": "running"},
			"InstanceType": "t2.micro",
		},
		"aws-ec2",
		"json",
	)
	if err != nil {
		t.Fatalf("Failed to assign schema: %v", err)
	}

	if schema != "aws.#EC2Instance" {
		t.Errorf("Expected aws.#EC2Instance, got %s", schema)
	}

	if confidence < 0.9 {
		t.Errorf("Expected high confidence (>0.9), got %f", confidence)
	}

	// Test engine info
	info, err := manager.GetEngineInfo()
	if err != nil {
		t.Fatalf("Failed to get engine info: %v", err)
	}

	if info.Name != "Legacy" {
		t.Errorf("Expected Legacy engine, got %s", info.Name)
	}

	// Test switching to Zygomys engine
	if err := manager.SwitchEngine("zygomys"); err != nil {
		t.Fatalf("Failed to switch to Zygomys engine: %v", err)
	}

	// Test schema assignment with Zygomys engine
	schema2, confidence2, err := manager.AssignSchema(
		map[string]interface{}{"test": "data"},
		"test",
		"json",
	)
	if err != nil {
		t.Fatalf("Failed to assign schema with Zygomys: %v", err)
	}

	if schema2 != "unknown.#CatchAll" {
		t.Errorf("Expected unknown.#CatchAll, got %s", schema2)
	}

	if confidence2 != 0.1 {
		t.Errorf("Expected 0.1 confidence, got %f", confidence2)
	}

	// Test engine info after switch
	info2, err := manager.GetEngineInfo()
	if err != nil {
		t.Fatalf("Failed to get engine info after switch: %v", err)
	}

	if info2.Name != "Zygomys" {
		t.Errorf("Expected Zygomys engine, got %s", info2.Name)
	}

	// Test switching back to legacy
	if err := manager.SwitchEngine("legacy"); err != nil {
		t.Fatalf("Failed to switch back to legacy engine: %v", err)
	}

	// Verify we're back to legacy
	info3, err := manager.GetEngineInfo()
	if err != nil {
		t.Fatalf("Failed to get engine info after switch back: %v", err)
	}

	if info3.Name != "Legacy" {
		t.Errorf("Expected Legacy engine after switch back, got %s", info3.Name)
	}
}

func TestLegacyRuleEngine(t *testing.T) {
	engine := NewLegacyRuleEngine()
	config := DefaultConfig()

	if err := engine.Initialize(config); err != nil {
		t.Fatalf("Failed to initialize legacy engine: %v", err)
	}
	defer engine.Close()

	tests := []struct {
		name           string
		data           interface{}
		origin         string
		format         string
		expectedSchema string
		minConfidence  float64
	}{
		{
			name: "AWS EC2 Instance",
			data: map[string]interface{}{
				"InstanceId":   "i-1234567890abcdef0",
				"State":       map[string]interface{}{"Name": "running"},
				"InstanceType": "t2.micro",
			},
			origin:         "aws-ec2",
			format:         "json",
			expectedSchema: "aws.#EC2Instance",
			minConfidence:  0.9,
		},
		{
			name: "Kubernetes Pod",
			data: map[string]interface{}{
				"kind":       "Pod",
				"apiVersion": "v1",
				"metadata":   map[string]interface{}{"name": "test-pod"},
			},
			origin:         "k8s",
			format:         "yaml",
			expectedSchema: "k8s.#Pod",
			minConfidence:  0.9,
		},
		{
			name: "Unknown data",
			data: map[string]interface{}{
				"unknown": "data",
			},
			origin:         "unknown",
			format:         "json",
			expectedSchema: "unknown.#CatchAll",
			minConfidence:  0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.AssignSchema(context.Background(), tt.data, tt.origin, tt.format)
			if err != nil {
				t.Fatalf("Failed to assign schema: %v", err)
			}

			if result.Schema != tt.expectedSchema {
				t.Errorf("Expected schema %s, got %s", tt.expectedSchema, result.Schema)
			}

			if result.Confidence < tt.minConfidence {
				t.Errorf("Expected confidence >= %f, got %f", tt.minConfidence, result.Confidence)
			}

			if result.Duration <= 0 {
				t.Error("Expected positive duration")
			}

			if result.RuleName == "" {
				t.Error("Expected non-empty rule name")
			}
		})
	}
}

func TestZygomysRuleEngine(t *testing.T) {
	engine := NewZygomysRuleEngine()
	config := DefaultConfig()

	if err := engine.Initialize(config); err != nil {
		t.Fatalf("Failed to initialize Zygomys engine: %v", err)
	}
	defer engine.Close()

	// Test basic functionality
	result, err := engine.AssignSchema(context.Background(), map[string]interface{}{"test": "data"}, "test", "json")
	if err != nil {
		t.Fatalf("Failed to assign schema: %v", err)
	}

	if result.Schema != "unknown.#CatchAll" {
		t.Errorf("Expected unknown.#CatchAll, got %s", result.Schema)
	}

	if result.Confidence != 0.1 {
		t.Errorf("Expected 0.1 confidence, got %f", result.Confidence)
	}

	if result.RuleName != "zygomys-basic-working" {
		t.Errorf("Expected zygomys-basic-working rule name, got %s", result.RuleName)
	}

	// Test engine info
	info := engine.GetInfo()
	if info.Name != "Zygomys" {
		t.Errorf("Expected Zygomys engine name, got %s", info.Name)
	}
}

func TestConfigManager(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pudl-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")
	manager := NewConfigManager(configPath)

	// Test loading default config (file doesn't exist)
	if err := manager.Load(); err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}

	config := manager.Get()
	if config.Type != "legacy" {
		t.Errorf("Expected legacy type, got %s", config.Type)
	}

	// Test updating config
	updates := map[string]interface{}{
		"type":       "zygomys",
		"timeout_ms": 10000,
		"debug":      true,
	}

	if err := manager.Update(updates); err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	config = manager.Get()
	if config.Type != "zygomys" {
		t.Errorf("Expected zygomys type after update, got %s", config.Type)
	}

	if config.TimeoutMS != 10000 {
		t.Errorf("Expected 10000 timeout, got %d", config.TimeoutMS)
	}

	if !config.Debug {
		t.Error("Expected debug to be true")
	}

	// Test saving and loading config
	if err := manager.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Create new manager and load saved config
	manager2 := NewConfigManager(configPath)
	if err := manager2.Load(); err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	config2 := manager2.Get()
	if config2.Type != "zygomys" {
		t.Errorf("Expected zygomys type after reload, got %s", config2.Type)
	}

	if config2.TimeoutMS != 10000 {
		t.Errorf("Expected 10000 timeout after reload, got %d", config2.TimeoutMS)
	}
}

func TestGlobalRegistry(t *testing.T) {
	// Test that both engines are registered
	engines := GlobalRegistry.List()
	
	expectedEngines := []string{"legacy", "zygomys"}
	for _, expected := range expectedEngines {
		found := false
		for _, engine := range engines {
			if engine == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected engine %s to be registered", expected)
		}
	}

	// Test creating engines
	legacyEngine, err := GlobalRegistry.Create("legacy")
	if err != nil {
		t.Fatalf("Failed to create legacy engine: %v", err)
	}

	if legacyEngine == nil {
		t.Error("Expected non-nil legacy engine")
	}

	zygomysEngine, err := GlobalRegistry.Create("zygomys")
	if err != nil {
		t.Fatalf("Failed to create Zygomys engine: %v", err)
	}

	if zygomysEngine == nil {
		t.Error("Expected non-nil Zygomys engine")
	}

	// Test creating unknown engine
	_, err = GlobalRegistry.Create("unknown")
	if err == nil {
		t.Error("Expected error when creating unknown engine")
	}
}
