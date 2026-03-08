package drift

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"pudl/internal/database"
	"pudl/internal/definition"
	"pudl/internal/executor"
	"pudl/internal/model"
	"pudl/internal/workflow"
)

// Checker performs drift detection by comparing declared definition state
// against live state from the latest artifact.
type Checker struct {
	defDisc   *definition.Discoverer
	modelDisc *model.Discoverer
	db        *database.CatalogDB
	executor  workflow.StepExecutor
	dataPath  string
}

// CheckOptions configures a drift check.
type CheckOptions struct {
	DefinitionName string
	Method         string // which method's artifact to compare; empty = auto-detect
	Refresh        bool   // re-execute the method before comparing
	Tags           map[string]string
}

// NewChecker creates a new drift checker.
func NewChecker(defDisc *definition.Discoverer, modelDisc *model.Discoverer, db *database.CatalogDB, exec workflow.StepExecutor, dataPath string) *Checker {
	return &Checker{
		defDisc:   defDisc,
		modelDisc: modelDisc,
		db:        db,
		executor:  exec,
		dataPath:  dataPath,
	}
}

// Check performs drift detection for a single definition.
func (c *Checker) Check(ctx context.Context, opts CheckOptions) (*DriftResult, error) {
	// Load definition
	def, err := c.defDisc.GetDefinition(opts.DefinitionName)
	if err != nil {
		return nil, fmt.Errorf("failed to load definition %q: %w", opts.DefinitionName, err)
	}

	// Determine method to use
	method := opts.Method
	if method == "" {
		method, err = c.findDefaultMethod(def.ModelRef)
		if err != nil {
			return nil, err
		}
	}

	// Optionally refresh by re-executing the method
	if opts.Refresh {
		_, err := c.executor.Run(ctx, executor.RunOptions{
			DefinitionName: opts.DefinitionName,
			MethodName:     method,
			Tags:           opts.Tags,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to refresh %s.%s: %w", opts.DefinitionName, method, err)
		}
	}

	// Load latest artifact
	entry, err := c.db.GetLatestArtifact(opts.DefinitionName, method)
	if err != nil {
		return &DriftResult{
			Definition: opts.DefinitionName,
			Method:     method,
			Status:     "unknown",
			Timestamp:  time.Now(),
		}, nil
	}

	// Read artifact JSON from StoredPath
	liveState, err := readArtifactJSON(entry.StoredPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact at %s: %w", entry.StoredPath, err)
	}

	// Build declared state from socket bindings
	declaredKeys := make(map[string]interface{})
	for k, v := range def.SocketBindings {
		declaredKeys[k] = v
	}

	// Compare
	diffs := Compare(declaredKeys, liveState)

	status := "clean"
	if len(diffs) > 0 {
		status = "drifted"
	}

	result := &DriftResult{
		Definition:   opts.DefinitionName,
		Method:       method,
		Status:       status,
		Timestamp:    time.Now(),
		DeclaredKeys: declaredKeys,
		LiveState:    liveState,
		Differences:  diffs,
	}

	// Save report
	store := NewReportStore(c.dataPath)
	if saveErr := store.Save(result); saveErr != nil {
		// Non-fatal: log but don't fail the check
		fmt.Fprintf(os.Stderr, "Warning: failed to save drift report: %v\n", saveErr)
	}

	return result, nil
}

// findDefaultMethod finds the first action method on the definition's model.
func (c *Checker) findDefaultMethod(modelRef string) (string, error) {
	m, err := c.modelDisc.GetModel(modelRef)
	if err != nil {
		return "", fmt.Errorf("failed to load model %q: %w", modelRef, err)
	}

	// Prefer "list" or "describe" if available, otherwise first action method
	for _, name := range []string{"list", "describe"} {
		if method, ok := m.Methods[name]; ok && method.Kind == "action" {
			return name, nil
		}
	}

	for name, method := range m.Methods {
		if method.Kind == "action" {
			return name, nil
		}
	}

	return "", fmt.Errorf("no action method found on model %q", modelRef)
}

// readArtifactJSON reads a JSON file and returns it as a map.
func readArtifactJSON(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return result, nil
}
