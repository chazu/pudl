package drift

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"pudl/internal/database"
	"pudl/internal/definition"
)

// Checker performs drift detection by comparing declared definition state
// against live state from the latest imported data.
type Checker struct {
	defDisc  *definition.Discoverer
	db       *database.CatalogDB
	dataPath string
}

// CheckOptions configures a drift check.
type CheckOptions struct {
	DefinitionName string
}

// NewChecker creates a new drift checker.
func NewChecker(defDisc *definition.Discoverer, db *database.CatalogDB, dataPath string) *Checker {
	return &Checker{
		defDisc:  defDisc,
		db:       db,
		dataPath: dataPath,
	}
}

// Check performs drift detection for a single definition.
func (c *Checker) Check(ctx context.Context, opts CheckOptions) (*DriftResult, error) {
	// Load definition
	def, err := c.defDisc.GetDefinition(opts.DefinitionName)
	if err != nil {
		return nil, fmt.Errorf("failed to load definition %q: %w", opts.DefinitionName, err)
	}

	// Load latest catalog entry for this definition
	entry, err := c.db.GetLatestArtifact(opts.DefinitionName, "")
	if err != nil {
		return &DriftResult{
			Definition: opts.DefinitionName,
			Status:     "unknown",
			Timestamp:  time.Now(),
		}, nil
	}

	// Read live state JSON from stored path
	liveState, err := readArtifactJSON(entry.StoredPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read data at %s: %w", entry.StoredPath, err)
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
