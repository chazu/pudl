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
	defDisc      *definition.Discoverer
	db           *database.CatalogDB
	dataPath     string
	originFilter string // empty means no filter
}

// CheckOptions configures a drift check.
type CheckOptions struct {
	DefinitionName string
	Method         string // which method's artifact to compare; empty = auto-detect
}

// NewChecker creates a new drift checker.
func NewChecker(defDisc *definition.Discoverer, db *database.CatalogDB, dataPath string) *Checker {
	return &Checker{
		defDisc:  defDisc,
		db:       db,
		dataPath: dataPath,
	}
}

// SetOriginFilter restricts catalog queries to entries matching the given origin.
// An empty string disables the filter.
func (c *Checker) SetOriginFilter(origin string) {
	c.originFilter = origin
}

// Check performs drift detection for a single definition.
func (c *Checker) Check(ctx context.Context, opts CheckOptions) (*DriftResult, error) {
	// Load definition
	def, err := c.defDisc.GetDefinition(opts.DefinitionName)
	if err != nil {
		return nil, fmt.Errorf("failed to load definition %q: %w", opts.DefinitionName, err)
	}

	// Load latest catalog entry for this definition.
	// Try observe results first (most recent actual state from mu),
	// then fall back to latest artifact.
	// When originFilter is set, use origin-scoped queries.
	var entry *database.CatalogEntry
	method := opts.Method
	if c.originFilter != "" {
		entry, err = c.db.GetLatestObserveByOrigin(opts.DefinitionName, c.originFilter)
		if err != nil || entry == nil {
			entry, err = c.db.GetLatestArtifactByOrigin(opts.DefinitionName, method, c.originFilter)
			if err != nil {
				return &DriftResult{
					Definition: opts.DefinitionName,
					Method:     method,
					Status:     "unknown",
					Timestamp:  time.Now(),
				}, nil
			}
		}
	} else {
		entry, err = c.db.GetLatestObserve(opts.DefinitionName)
		if err != nil || entry == nil {
			entry, err = c.db.GetLatestArtifact(opts.DefinitionName, method)
			if err != nil {
				return &DriftResult{
					Definition: opts.DefinitionName,
					Method:     method,
					Status:     "unknown",
					Timestamp:  time.Now(),
				}, nil
			}
		}
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

	// Update convergence status in catalog
	if err := c.db.UpdateStatus(opts.DefinitionName, status); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update status: %v\n", err)
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
