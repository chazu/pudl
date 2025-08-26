package lister

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Lister handles data listing and querying operations
type Lister struct {
	dataPath string
}

// FilterOptions contains filtering criteria for listing data
type FilterOptions struct {
	Schema string // Filter by CUE schema
	Origin string // Filter by data origin
	Format string // Filter by file format
}

// DisplayOptions contains display preferences for listing data
type DisplayOptions struct {
	Verbose bool   // Show detailed information
	Limit   int    // Maximum number of results
	SortBy  string // Field to sort by
	Reverse bool   // Reverse sort order
}

// ListEntry represents a single entry in the list results
type ListEntry struct {
	ID              string    `json:"id"`
	StoredPath      string    `json:"stored_path"`
	MetadataPath    string    `json:"metadata_path"`
	ImportTimestamp string    `json:"import_timestamp"`
	ParsedTimestamp time.Time `json:"-"` // For sorting
	Format          string    `json:"format"`
	Origin          string    `json:"origin"`
	Schema          string    `json:"schema"`
	Confidence      float64   `json:"confidence"`
	RecordCount     int       `json:"record_count"`
	SizeBytes       int64     `json:"size_bytes"`
}

// ListResults contains the results of a list operation
type ListResults struct {
	Entries       []ListEntry `json:"entries"`
	TotalEntries  int         `json:"total_entries"`
	TotalSize     int64       `json:"total_size"`
	TotalRecords  int         `json:"total_records"`
	UniqueSchemas []string    `json:"unique_schemas"`
	UniqueOrigins []string    `json:"unique_origins"`
	UniqueFormats []string    `json:"unique_formats"`
}

// CatalogEntry represents an entry from the catalog file
// This mirrors the structure from internal/importer/metadata.go
type CatalogEntry struct {
	ID              string  `json:"id"`
	StoredPath      string  `json:"stored_path"`
	MetadataPath    string  `json:"metadata_path"`
	ImportTimestamp string  `json:"import_timestamp"`
	Format          string  `json:"format"`
	Origin          string  `json:"origin"`
	Schema          string  `json:"schema"`
	Confidence      float64 `json:"confidence"`
	RecordCount     int     `json:"record_count"`
	SizeBytes       int64   `json:"size_bytes"`
}

// Catalog represents the main data catalog
type Catalog struct {
	Entries     []CatalogEntry `json:"entries"`
	LastUpdated string         `json:"last_updated"`
	Version     string         `json:"version"`
}

// New creates a new Lister instance
func New(dataPath string) *Lister {
	return &Lister{
		dataPath: dataPath,
	}
}

// ListData lists and filters data based on the provided criteria
func (l *Lister) ListData(filters FilterOptions, displayOpts DisplayOptions) (*ListResults, error) {
	// Load catalog
	catalog, err := l.loadCatalog()
	if err != nil {
		return nil, fmt.Errorf("failed to load catalog: %w", err)
	}

	// Convert catalog entries to list entries and apply filters
	var filteredEntries []ListEntry
	for _, entry := range catalog.Entries {
		// Apply filters
		if !l.matchesFilters(entry, filters) {
			continue
		}

		// Parse timestamp for sorting
		parsedTime, err := time.Parse(time.RFC3339, entry.ImportTimestamp)
		if err != nil {
			// If parsing fails, use zero time
			parsedTime = time.Time{}
		}

		listEntry := ListEntry{
			ID:              entry.ID,
			StoredPath:      entry.StoredPath,
			MetadataPath:    entry.MetadataPath,
			ImportTimestamp: entry.ImportTimestamp,
			ParsedTimestamp: parsedTime,
			Format:          entry.Format,
			Origin:          entry.Origin,
			Schema:          entry.Schema,
			Confidence:      entry.Confidence,
			RecordCount:     entry.RecordCount,
			SizeBytes:       entry.SizeBytes,
		}

		filteredEntries = append(filteredEntries, listEntry)
	}

	// Sort entries
	l.sortEntries(filteredEntries, displayOpts.SortBy, displayOpts.Reverse)

	// Apply limit
	totalEntries := len(filteredEntries)
	if displayOpts.Limit > 0 && displayOpts.Limit < len(filteredEntries) {
		filteredEntries = filteredEntries[:displayOpts.Limit]
	}

	// Calculate summary statistics
	results := &ListResults{
		Entries:      filteredEntries,
		TotalEntries: totalEntries,
	}

	// Calculate totals and unique values from all filtered entries (not just limited ones)
	allFilteredEntries := filteredEntries
	if displayOpts.Limit > 0 && displayOpts.Limit < totalEntries {
		// Reload all filtered entries for statistics
		allFilteredEntries = []ListEntry{}
		for _, entry := range catalog.Entries {
			if l.matchesFilters(entry, filters) {
				parsedTime, _ := time.Parse(time.RFC3339, entry.ImportTimestamp)
				listEntry := ListEntry{
					ID:              entry.ID,
					StoredPath:      entry.StoredPath,
					MetadataPath:    entry.MetadataPath,
					ImportTimestamp: entry.ImportTimestamp,
					ParsedTimestamp: parsedTime,
					Format:          entry.Format,
					Origin:          entry.Origin,
					Schema:          entry.Schema,
					Confidence:      entry.Confidence,
					RecordCount:     entry.RecordCount,
					SizeBytes:       entry.SizeBytes,
				}
				allFilteredEntries = append(allFilteredEntries, listEntry)
			}
		}
	}

	l.calculateSummaryStats(results, allFilteredEntries)

	return results, nil
}

// matchesFilters checks if a catalog entry matches the given filters
func (l *Lister) matchesFilters(entry CatalogEntry, filters FilterOptions) bool {
	// Schema filter
	if filters.Schema != "" && !strings.Contains(strings.ToLower(entry.Schema), strings.ToLower(filters.Schema)) {
		return false
	}

	// Origin filter
	if filters.Origin != "" && !strings.Contains(strings.ToLower(entry.Origin), strings.ToLower(filters.Origin)) {
		return false
	}

	// Format filter
	if filters.Format != "" && !strings.EqualFold(entry.Format, filters.Format) {
		return false
	}

	return true
}

// sortEntries sorts the entries based on the specified field and order
func (l *Lister) sortEntries(entries []ListEntry, sortBy string, reverse bool) {
	sort.Slice(entries, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "timestamp":
			less = entries[i].ParsedTimestamp.Before(entries[j].ParsedTimestamp)
		case "size":
			less = entries[i].SizeBytes < entries[j].SizeBytes
		case "records":
			less = entries[i].RecordCount < entries[j].RecordCount
		case "schema":
			less = strings.ToLower(entries[i].Schema) < strings.ToLower(entries[j].Schema)
		case "origin":
			less = strings.ToLower(entries[i].Origin) < strings.ToLower(entries[j].Origin)
		case "format":
			less = strings.ToLower(entries[i].Format) < strings.ToLower(entries[j].Format)
		default:
			// Default to timestamp
			less = entries[i].ParsedTimestamp.Before(entries[j].ParsedTimestamp)
		}

		if reverse {
			return !less
		}
		return less
	})
}

// calculateSummaryStats calculates summary statistics for the results
func (l *Lister) calculateSummaryStats(results *ListResults, entries []ListEntry) {
	schemaSet := make(map[string]bool)
	originSet := make(map[string]bool)
	formatSet := make(map[string]bool)

	for _, entry := range entries {
		results.TotalSize += entry.SizeBytes
		results.TotalRecords += entry.RecordCount

		schemaSet[entry.Schema] = true
		originSet[entry.Origin] = true
		formatSet[entry.Format] = true
	}

	// Convert sets to sorted slices
	for schema := range schemaSet {
		results.UniqueSchemas = append(results.UniqueSchemas, schema)
	}
	sort.Strings(results.UniqueSchemas)

	for origin := range originSet {
		results.UniqueOrigins = append(results.UniqueOrigins, origin)
	}
	sort.Strings(results.UniqueOrigins)

	for format := range formatSet {
		results.UniqueFormats = append(results.UniqueFormats, format)
	}
	sort.Strings(results.UniqueFormats)
}

// loadCatalog loads the catalog from disk
func (l *Lister) loadCatalog() (*Catalog, error) {
	catalogPath := l.getCatalogPath()

	// Check if catalog exists
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		return &Catalog{
			Entries: []CatalogEntry{},
			Version: "1.0",
		}, nil
	}

	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog file: %w", err)
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse catalog file: %w", err)
	}

	return &catalog, nil
}

// getCatalogPath returns the path to the catalog file
func (l *Lister) getCatalogPath() string {
	return filepath.Join(l.dataPath, "catalog", "inventory.json")
}

// FindEntry finds a specific entry by ID
func (l *Lister) FindEntry(id string) (*ListEntry, error) {
	// Load catalog
	catalog, err := l.loadCatalog()
	if err != nil {
		return nil, fmt.Errorf("failed to load catalog: %w", err)
	}

	// Search for the entry
	for _, entry := range catalog.Entries {
		if entry.ID == id {
			// Parse timestamp
			parsedTime, err := time.Parse(time.RFC3339, entry.ImportTimestamp)
			if err != nil {
				parsedTime = time.Time{}
			}

			listEntry := &ListEntry{
				ID:              entry.ID,
				StoredPath:      entry.StoredPath,
				MetadataPath:    entry.MetadataPath,
				ImportTimestamp: entry.ImportTimestamp,
				ParsedTimestamp: parsedTime,
				Format:          entry.Format,
				Origin:          entry.Origin,
				Schema:          entry.Schema,
				Confidence:      entry.Confidence,
				RecordCount:     entry.RecordCount,
				SizeBytes:       entry.SizeBytes,
			}

			return listEntry, nil
		}
	}

	return nil, nil // Entry not found
}
