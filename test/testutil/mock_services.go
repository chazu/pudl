package testutil

import (
	"fmt"
	"sync"
	"time"

	"pudl/internal/database"
	"pudl/internal/inference"
)

// MockProgressReporter implements a mock progress reporter for testing
type MockProgressReporter struct {
	mu       sync.Mutex
	messages []string
	progress []int
}

// NewMockProgressReporter creates a new mock progress reporter
func NewMockProgressReporter() *MockProgressReporter {
	return &MockProgressReporter{
		messages: make([]string, 0),
		progress: make([]int, 0),
	}
}

// Report records a progress message
func (m *MockProgressReporter) Report(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, message)
}

// ReportProgress records progress percentage
func (m *MockProgressReporter) ReportProgress(percent int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress = append(m.progress, percent)
}

// GetMessages returns all recorded messages
func (m *MockProgressReporter) GetMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.messages))
	copy(result, m.messages)
	return result
}

// GetProgress returns all recorded progress values
func (m *MockProgressReporter) GetProgress() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]int, len(m.progress))
	copy(result, m.progress)
	return result
}

// MockSchemaInferrer implements a mock schema inferrer for testing
type MockSchemaInferrer struct {
	results map[string]*inference.InferenceResult
}

// NewMockSchemaInferrer creates a new mock schema inferrer
func NewMockSchemaInferrer() *MockSchemaInferrer {
	return &MockSchemaInferrer{
		results: make(map[string]*inference.InferenceResult),
	}
}

// AddResult adds a mock result for testing
func (m *MockSchemaInferrer) AddResult(pattern string, result *inference.InferenceResult) {
	m.results[pattern] = result
}

// Infer returns a mock schema assignment based on registered results
func (m *MockSchemaInferrer) Infer(filename string, data interface{}) *inference.InferenceResult {
	// Simple pattern matching for testing
	for pattern, result := range m.results {
		if contains(filename, pattern) {
			return result
		}
	}

	// Default assignment for unknown files
	return &inference.InferenceResult{
		Schema:     "unknown.#CatchAll",
		Confidence: 0.5,
		Reason:     "No specific pattern matched, using default schema",
	}
}

// contains is a simple helper for pattern matching
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}

// MockCatalog implements a mock catalog for testing database operations
type MockCatalog struct {
	mu      sync.RWMutex
	entries map[string]*database.CatalogEntry
	nextID  int
}

// NewMockCatalog creates a new mock catalog
func NewMockCatalog() *MockCatalog {
	return &MockCatalog{
		entries: make(map[string]*database.CatalogEntry),
		nextID:  1,
	}
}

// AddEntry adds an entry to the mock catalog
func (m *MockCatalog) AddEntry(entry database.CatalogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.ID == "" {
		entry.ID = fmt.Sprintf("mock-entry-%d", m.nextID)
		m.nextID++
	}

	if entry.ImportTimestamp.IsZero() {
		entry.ImportTimestamp = time.Now()
	}

	m.entries[entry.StoredPath] = &entry
	return nil
}

// GetEntry retrieves an entry from the mock catalog by ID
func (m *MockCatalog) GetEntry(id string) (*database.CatalogEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, entry := range m.entries {
		if entry.ID == id {
			// Return a copy to avoid race conditions
			entryCopy := *entry
			return &entryCopy, nil
		}
	}

	return nil, fmt.Errorf("entry not found: %s", id)
}

// QueryEntries returns entries matching the given criteria
func (m *MockCatalog) QueryEntries(filters database.FilterOptions, options database.QueryOptions) (*database.QueryResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []database.CatalogEntry

	for _, entry := range m.entries {
		if m.matchesFilter(entry, &filters) {
			entryCopy := *entry
			results = append(results, entryCopy)
		}
	}

	// Apply limit and offset
	totalCount := len(m.entries)
	filteredCount := len(results)

	if options.Offset > 0 && options.Offset < len(results) {
		results = results[options.Offset:]
	} else if options.Offset >= len(results) {
		results = []database.CatalogEntry{}
	}

	if options.Limit > 0 && options.Limit < len(results) {
		results = results[:options.Limit]
	}

	return &database.QueryResult{
		Entries:       results,
		TotalCount:    totalCount,
		FilteredCount: filteredCount,
	}, nil
}

// matchesFilter checks if an entry matches the given filter
func (m *MockCatalog) matchesFilter(entry *database.CatalogEntry, filter *database.FilterOptions) bool {
	if filter == nil {
		return true
	}

	if filter.Schema != "" && entry.Schema != filter.Schema {
		return false
	}

	if filter.Origin != "" && entry.Origin != filter.Origin {
		return false
	}

	if filter.Format != "" && entry.Format != filter.Format {
		return false
	}

	return true
}

// UpdateEntry updates an existing entry in the mock catalog
func (m *MockCatalog) UpdateEntry(entry database.CatalogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[entry.StoredPath]; !exists {
		return fmt.Errorf("entry not found: %s", entry.StoredPath)
	}

	m.entries[entry.StoredPath] = &entry
	return nil
}

// DeleteEntry removes an entry from the mock catalog
func (m *MockCatalog) DeleteEntry(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for path, entry := range m.entries {
		if entry.ID == id {
			delete(m.entries, path)
			return nil
		}
	}

	return fmt.Errorf("entry not found: %s", id)
}

// GetEntryCount returns the number of entries in the mock catalog
func (m *MockCatalog) GetEntryCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// Clear removes all entries from the mock catalog
func (m *MockCatalog) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[string]*database.CatalogEntry)
	m.nextID = 1
}

// MockFileSystem implements a mock file system for testing
type MockFileSystem struct {
	files map[string][]byte
	dirs  map[string]bool
}

// NewMockFileSystem creates a new mock file system
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

// WriteFile writes a file to the mock file system
func (m *MockFileSystem) WriteFile(path string, content []byte) {
	m.files[path] = content
}

// ReadFile reads a file from the mock file system
func (m *MockFileSystem) ReadFile(path string) ([]byte, error) {
	content, exists := m.files[path]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return content, nil
}

// Exists checks if a file exists in the mock file system
func (m *MockFileSystem) Exists(path string) bool {
	_, exists := m.files[path]
	return exists || m.dirs[path]
}

// CreateDir creates a directory in the mock file system
func (m *MockFileSystem) CreateDir(path string) {
	m.dirs[path] = true
}

// ListFiles returns all files in the mock file system
func (m *MockFileSystem) ListFiles() []string {
	var files []string
	for path := range m.files {
		files = append(files, path)
	}
	return files
}
