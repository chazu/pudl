package drift

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ReportStore manages drift report persistence.
type ReportStore struct {
	dataPath string
}

// NewReportStore creates a new report store.
func NewReportStore(dataPath string) *ReportStore {
	return &ReportStore{dataPath: dataPath}
}

// Save writes a drift result to disk.
func (s *ReportStore) Save(result *DriftResult) error {
	dir := s.reportDir(result.Definition)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create drift directory: %w", err)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize drift report: %w", err)
	}

	filename := result.Timestamp.Format("20060102T150405") + ".json"
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write drift report: %w", err)
	}

	return nil
}

// List returns all report timestamps for a definition, sorted newest first.
func (s *ReportStore) List(definitionName string) ([]string, error) {
	dir := s.reportDir(definitionName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read drift directory: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(entry.Name(), ".json"))
	}

	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids, nil
}

// Get loads a specific drift report by definition and timestamp ID.
func (s *ReportStore) Get(definitionName, id string) (*DriftResult, error) {
	path := filepath.Join(s.reportDir(definitionName), id+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("drift report %q not found for %q", id, definitionName)
		}
		return nil, fmt.Errorf("failed to read drift report: %w", err)
	}

	var result DriftResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse drift report: %w", err)
	}

	return &result, nil
}

// GetLatest returns the most recent drift report for a definition.
func (s *ReportStore) GetLatest(definitionName string) (*DriftResult, error) {
	ids, err := s.List(definitionName)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no drift reports found for %q", definitionName)
	}

	return s.Get(definitionName, ids[0])
}

// ListDefinitions returns the names of all definitions that have drift reports.
func (s *ReportStore) ListDefinitions() ([]string, error) {
	driftDir := filepath.Join(s.dataPath, ".drift")
	entries, err := os.ReadDir(driftDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read drift directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// reportDir returns the directory for a definition's drift reports.
func (s *ReportStore) reportDir(definitionName string) string {
	return filepath.Join(s.dataPath, ".drift", definitionName)
}

// FormatTimestamp formats a report timestamp for display.
func FormatTimestamp(ts time.Time) string {
	return ts.Format("2006-01-02 15:04:05")
}
