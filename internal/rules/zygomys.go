package rules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/glycerine/zygomys/zygo"
)

// ZygomysRuleEngine implements the RuleEngine interface using Zygomys Lisp
type ZygomysRuleEngine struct {
	mu          sync.RWMutex
	config      *Config
	info        *EngineInfo
	interpreter *zygo.Zlisp
	ruleFiles   map[string]string // rule name -> file path
	initialized bool
}

// NewZygomysRuleEngine creates a new Zygomys rule engine
func NewZygomysRuleEngine() RuleEngine {
	return &ZygomysRuleEngine{
		info: &EngineInfo{
			Name:        "Zygomys",
			Version:     "1.0.0",
			Description: "Lisp-based rule engine with user-extensible schema assignment rules",
			RuleCount:   0, // Will be updated when rules are loaded
		},
		ruleFiles: make(map[string]string),
	}
}

// Initialize sets up the Zygomys rule engine
func (e *ZygomysRuleEngine) Initialize(config *Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if config == nil {
		return ErrConfigurationError("config cannot be nil")
	}

	e.config = config

	// Create new Zygomys interpreter
	interpreter := zygo.NewZlisp()
	e.interpreter = interpreter

	// Set up the interpreter environment
	if err := e.setupEnvironment(); err != nil {
		return ErrInitializationFailed(err)
	}

	// Load built-in rules
	if err := e.loadBuiltinRules(); err != nil {
		return ErrInitializationFailed(err)
	}

	// Load custom rules if rule directory exists
	if config.RuleDir != "" {
		if err := e.loadCustomRules(); err != nil {
			// Don't fail initialization if custom rules fail to load
			// Just log a warning (in a real implementation, we'd use proper logging)
			if config.Debug {
				fmt.Printf("Warning: Failed to load custom rules: %v\n", err)
			}
		}
	}

	e.initialized = true
	return nil
}

// AssignSchema determines the best schema for given data using Zygomys rules
func (e *ZygomysRuleEngine) AssignSchema(ctx context.Context, data interface{}, origin, format string) (*Result, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.initialized {
		return nil, ErrInitializationFailed(fmt.Errorf("engine not initialized"))
	}

	start := time.Now()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ErrExecutionTimeout(e.config.TimeoutMS)
	default:
	}

	// For now, just return a basic result without executing Lisp rules
	// This confirms the Zygomys engine infrastructure works
	return &Result{
		Schema:     "unknown.#CatchAll",
		Confidence: 0.1,
		RuleName:   "zygomys-basic-working",
		Duration:   time.Since(start),
		Metadata: map[string]interface{}{
			"engine": "zygomys",
			"origin": origin,
			"format": format,
			"note":   "basic Zygomys engine working - Lisp rules to be added later",
		},
	}, nil
}

// GetInfo returns information about the Zygomys rule engine
func (e *ZygomysRuleEngine) GetInfo() *EngineInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.info
}

// Close releases any resources held by the engine
func (e *ZygomysRuleEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.interpreter != nil {
		// Zygomys doesn't have an explicit close method
		e.interpreter = nil
	}

	e.initialized = false
	return nil
}

// setupEnvironment sets up the Zygomys interpreter environment with helper functions
func (e *ZygomysRuleEngine) setupEnvironment() error {
	// For now, we'll use a simplified approach and rely on built-in Zygomys functions
	// The complex helper functions can be added later once we understand the API better
	return nil
}

// loadBuiltinRules loads the built-in schema assignment rules
func (e *ZygomysRuleEngine) loadBuiltinRules() error {
	// Skip loading any Lisp rules for now - just test basic interpreter functionality
	_, err := e.interpreter.EvalString(`(+ 1 2)`)
	if err != nil {
		return fmt.Errorf("failed basic arithmetic test: %w", err)
	}

	// Update rule count
	e.info.RuleCount = 1 // Basic catchall rule

	return nil
}

// loadCustomRules loads custom rules from the rule directory
func (e *ZygomysRuleEngine) loadCustomRules() error {
	if e.config.RuleDir == "" {
		return nil
	}

	// Check if rule directory exists
	if _, err := os.Stat(e.config.RuleDir); os.IsNotExist(err) {
		// Directory doesn't exist, create it
		if err := os.MkdirAll(e.config.RuleDir, 0755); err != nil {
			return fmt.Errorf("failed to create rule directory: %w", err)
		}
		return nil
	}

	// Look for .zy files in the rule directory
	pattern := filepath.Join(e.config.RuleDir, "*.zy")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find rule files: %w", err)
	}

	// Load each rule file
	for _, file := range files {
		if err := e.loadRuleFile(file); err != nil {
			if e.config.Debug {
				fmt.Printf("Warning: Failed to load rule file %s: %v\n", file, err)
			}
			continue
		}
	}

	return nil
}

// loadRuleFile loads a single rule file
func (e *ZygomysRuleEngine) loadRuleFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read rule file: %w", err)
	}

	_, err = e.interpreter.EvalString(string(content))
	if err != nil {
		return fmt.Errorf("failed to evaluate rule file: %w", err)
	}

	// Store the rule file path
	ruleName := filepath.Base(filePath)
	e.ruleFiles[ruleName] = filePath

	return nil
}

// convertToZygoData converts Go data to Zygomys-compatible format
// This is a placeholder for future implementation
func (e *ZygomysRuleEngine) convertToZygoData(data interface{}) (zygo.Sexp, error) {
	// For now, just return nil - this will be implemented later
	return zygo.SexpNull, nil
}

// parseZygoResult parses the result from Zygomys execution
func (e *ZygomysRuleEngine) parseZygoResult(result zygo.Sexp, duration time.Duration) (*Result, error) {
	// Handle simple string result
	if str, ok := result.(*zygo.SexpStr); ok {
		return &Result{
			Schema:     str.S,
			Confidence: 0.1,
			RuleName:   "zygomys-string-result",
			Duration:   duration,
			Metadata: map[string]interface{}{
				"engine": "zygomys",
			},
		}, nil
	}

	// Fallback for any other result type
	return &Result{
		Schema:     "unknown.#CatchAll",
		Confidence: 0.1,
		RuleName:   "zygomys-fallback",
		Duration:   duration,
		Metadata: map[string]interface{}{
			"engine": "zygomys",
			"error":  "unexpected result format",
		},
	}, nil


}

// ErrExecutionFailed creates an error for rule execution failures
func ErrExecutionFailed(cause error) *RuleEngineError {
	return NewRuleEngineErrorWithCause(ErrorCodeExecutionFailed, "rule execution failed", cause)
}

// Register the Zygomys rule engine with the global registry
func init() {
	GlobalRegistry.Register("zygomys", NewZygomysRuleEngine)
}
