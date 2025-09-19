package lister

import (
	"fmt"
	"time"

	"pudl/internal/database"
	"pudl/internal/errors"
)

// Lister handles data listing and querying operations
type Lister struct {
	dataPath  string
	catalogDB *database.CatalogDB
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
func New(dataPath string) (*Lister, error) {
	// Initialize catalog database
	catalogDB, err := database.NewCatalogDB(dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize catalog database: %w", err)
	}

	lister := &Lister{
		dataPath:  dataPath,
		catalogDB: catalogDB,
	}

	// Check if migration is needed and perform it
	if err := lister.performMigrationIfNeeded(); err != nil {
		return nil, fmt.Errorf("failed to perform catalog migration: %w", err)
	}

	return lister, nil
}

// Close closes the lister and its database connections
func (l *Lister) Close() error {
	if l.catalogDB != nil {
		return l.catalogDB.Close()
	}
	return nil
}

// performMigrationIfNeeded checks if migration is needed and performs it
func (l *Lister) performMigrationIfNeeded() error {
	needed, err := l.catalogDB.CheckMigrationNeeded()
	if err != nil {
		return fmt.Errorf("failed to check migration status: %w", err)
	}

	if needed {
		fmt.Println("Migrating catalog from JSON to SQLite...")
		result, err := l.catalogDB.MigrateFromJSON()
		if err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}

		fmt.Printf("Migration completed: %d entries migrated, %d skipped\n",
			result.MigratedEntries, result.SkippedEntries)

		if len(result.Errors) > 0 {
			fmt.Printf("Migration warnings: %d\n", len(result.Errors))
			for _, errMsg := range result.Errors {
				fmt.Printf("  - %s\n", errMsg)
			}
		}
	}

	return nil
}

// ListData lists and filters data based on the provided criteria
func (l *Lister) ListData(filters FilterOptions, displayOpts DisplayOptions) (*ListResults, error) {
	// Convert display options to database query options
	queryOpts := database.QueryOptions{
		Limit:   displayOpts.Limit,
		Offset:  0, // TODO: Add pagination support to DisplayOptions
		SortBy:  displayOpts.SortBy,
		Reverse: displayOpts.Reverse,
	}

	// Convert filters to database filters
	dbFilters := database.FilterOptions{
		Schema: filters.Schema,
		Origin: filters.Origin,
		Format: filters.Format,
	}

	// Query database
	queryResult, err := l.catalogDB.QueryEntries(dbFilters, queryOpts)
	if err != nil {
		return nil, err // Already a PUDLError from database
	}

	// Convert database entries to list entries
	var listEntries []ListEntry
	for _, dbEntry := range queryResult.Entries {
		listEntry := ListEntry{
			ID:              dbEntry.ID,
			StoredPath:      dbEntry.StoredPath,
			MetadataPath:    dbEntry.MetadataPath,
			ImportTimestamp: dbEntry.ImportTimestamp.Format(time.RFC3339),
			ParsedTimestamp: dbEntry.ImportTimestamp,
			Format:          dbEntry.Format,
			Origin:          dbEntry.Origin,
			Schema:          dbEntry.Schema,
			Confidence:      dbEntry.Confidence,
			RecordCount:     dbEntry.RecordCount,
			SizeBytes:       dbEntry.SizeBytes,
		}
		listEntries = append(listEntries, listEntry)
	}

	// Create results
	results := &ListResults{
		Entries:      listEntries,
		TotalEntries: queryResult.FilteredCount, // Use filtered count as total for display
	}

	// Calculate summary statistics
	l.calculateSummaryStats(results, listEntries)

	return results, nil
}



// calculateSummaryStats calculates summary statistics for the results
func (l *Lister) calculateSummaryStats(results *ListResults, entries []ListEntry) {
	// Calculate totals from the provided entries
	for _, entry := range entries {
		results.TotalSize += entry.SizeBytes
		results.TotalRecords += entry.RecordCount
	}

	// Get unique values from database (more efficient than from filtered results)
	if schemas, err := l.catalogDB.GetUniqueValues("schema"); err == nil {
		results.UniqueSchemas = schemas
	}

	if origins, err := l.catalogDB.GetUniqueValues("origin"); err == nil {
		results.UniqueOrigins = origins
	}

	if formats, err := l.catalogDB.GetUniqueValues("format"); err == nil {
		results.UniqueFormats = formats
	}
}



// FindEntry finds a specific entry by ID
func (l *Lister) FindEntry(id string) (*ListEntry, error) {
	// Query database for the specific entry
	dbEntry, err := l.catalogDB.GetEntry(id)
	if err != nil {
		// Convert database error to user-friendly error
		if errors.GetErrorCode(err) == errors.ErrCodeNotFound {
			return nil, errors.NewInputError(
				fmt.Sprintf("Entry not found: %s", id),
				"Check the entry ID with 'pudl list'",
				"Ensure you're using the correct entry identifier")
		}
		return nil, err // Already a PUDLError from database
	}

	// Convert database entry to list entry
	listEntry := &ListEntry{
		ID:              dbEntry.ID,
		StoredPath:      dbEntry.StoredPath,
		MetadataPath:    dbEntry.MetadataPath,
		ImportTimestamp: dbEntry.ImportTimestamp.Format(time.RFC3339),
		ParsedTimestamp: dbEntry.ImportTimestamp,
		Format:          dbEntry.Format,
		Origin:          dbEntry.Origin,
		Schema:          dbEntry.Schema,
		Confidence:      dbEntry.Confidence,
		RecordCount:     dbEntry.RecordCount,
		SizeBytes:       dbEntry.SizeBytes,
	}

	return listEntry, nil
}
