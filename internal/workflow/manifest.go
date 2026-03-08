package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manifest records the result of a workflow run.
type Manifest struct {
	WorkflowName string                  `json:"workflow_name"`
	RunID        string                  `json:"run_id"`
	Status       string                  `json:"status"` // success, failed, partial
	StartTime    time.Time               `json:"start_time"`
	EndTime      time.Time               `json:"end_time"`
	DurationSecs float64                 `json:"duration_secs"`
	Steps        map[string]StepManifest `json:"steps"`
}

// StepManifest records the result of a single workflow step.
type StepManifest struct {
	StepName       string  `json:"step_name"`
	DefinitionName string  `json:"definition_name"`
	MethodName     string  `json:"method_name"`
	Status         string  `json:"status"` // success, failed, skipped
	Error          string  `json:"error,omitempty"`
	ArtifactID     string  `json:"artifact_id,omitempty"`
	StartTime      string  `json:"start_time"`
	EndTime        string  `json:"end_time"`
	DurationSecs   float64 `json:"duration_secs"`
	Attempt        int     `json:"attempt"`
}

// ManifestStore manages run manifest persistence.
type ManifestStore struct {
	dataPath string // root data directory (e.g. ~/.pudl/data)
}

// NewManifestStore creates a new manifest store.
func NewManifestStore(dataPath string) *ManifestStore {
	return &ManifestStore{dataPath: dataPath}
}

// Save writes a manifest to disk.
func (s *ManifestStore) Save(m *Manifest) error {
	dir := s.runDir(m.WorkflowName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create runs directory: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	path := filepath.Join(dir, m.RunID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// List returns all run IDs for a workflow, sorted newest first.
func (s *ManifestStore) List(workflowName string) ([]string, error) {
	dir := s.runDir(workflowName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read runs directory: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(entry.Name(), ".json"))
	}

	// Sort newest first (run IDs are timestamp-based)
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids, nil
}

// Get loads a specific manifest by workflow name and run ID.
func (s *ManifestStore) Get(workflowName, runID string) (*Manifest, error) {
	path := filepath.Join(s.runDir(workflowName), runID+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("run %q not found for workflow %q", runID, workflowName)
		}
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &m, nil
}

// runDir returns the directory for a workflow's run manifests.
func (s *ManifestStore) runDir(workflowName string) string {
	return filepath.Join(s.dataPath, ".runs", workflowName)
}
