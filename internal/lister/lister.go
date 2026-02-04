package lister

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pudl/internal/database"
	"pudl/internal/errors"
	"pudl/internal/idgen"
)

// Lister handles data listing and querying operations
type Lister struct {
	dataPath  string
	catalogDB *database.CatalogDB
}

// FilterOptions contains filtering criteria for listing data
type FilterOptions struct {
	Schema         string // Filter by CUE schema
	Origin         string // Filter by data origin
	Format         string // Filter by file format
	CollectionID   string // Filter by collection ID
	CollectionType string // Filter by collection type ('collection', 'item')
	ItemID         string // Filter by item ID
}

// DisplayOptions contains display preferences for listing data
type DisplayOptions struct {
	Verbose bool   // Show detailed information
	Limit   int    // Maximum number of results
	SortBy  string // Field to sort by
	Reverse bool   // Reverse sort order
	Page    int    // Page number (1-based)
	PerPage int    // Results per page
}

// ListEntry represents a single entry in the list results
type ListEntry struct {
	ID              string    `json:"id"`
	Proquint        string    `json:"proquint"` // Human-friendly ID derived from content hash
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
	// Collection fields
	CollectionID   *string `json:"collection_id,omitempty"`
	ItemIndex      *int    `json:"item_index,omitempty"`
	CollectionType *string `json:"collection_type,omitempty"`
	ItemID         *string `json:"item_id,omitempty"`
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
	TotalPages    int         `json:"total_pages"`
	CurrentPage   int         `json:"current_page"`
}

// DeleteResult contains the result of a delete operation
type DeleteResult struct {
	Success             bool     `json:"success"`
	EntryID             string   `json:"entry_id"`
	Proquint            string   `json:"proquint"`
	DataFileDeleted     bool     `json:"data_file_deleted"`
	MetadataFileDeleted bool     `json:"metadata_file_deleted"`
	ItemsDeleted        int      `json:"items_deleted,omitempty"`
	DeletedItemIDs      []string `json:"deleted_item_ids,omitempty"`
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
	// Initialize catalog database with config directory
	// We need to derive the config directory from the data path
	// dataPath is typically ~/.pudl/data, so config dir is ~/.pudl
	configDir := filepath.Dir(dataPath)
	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize catalog database: %w", err)
	}

	lister := &Lister{
		dataPath:  dataPath,
		catalogDB: catalogDB,
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

// ListData lists and filters data based on the provided criteria
func (l *Lister) ListData(filters FilterOptions, displayOpts DisplayOptions) (*ListResults, error) {
	// Determine page and per-page values
	page := displayOpts.Page
	perPage := displayOpts.PerPage

	// Default values if not set
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = displayOpts.Limit
		if perPage < 1 {
			perPage = 20 // Default per-page
		}
	}

	// Calculate offset from page: offset = (page - 1) * perPage
	offset := (page - 1) * perPage

	// Convert display options to database query options
	queryOpts := database.QueryOptions{
		Limit:   perPage,
		Offset:  offset,
		SortBy:  displayOpts.SortBy,
		Reverse: displayOpts.Reverse,
	}

	// Convert filters to database filters
	dbFilters := database.FilterOptions{
		Schema:         filters.Schema,
		Origin:         filters.Origin,
		Format:         filters.Format,
		CollectionID:   filters.CollectionID,
		CollectionType: filters.CollectionType,
		ItemID:         filters.ItemID,
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
			Proquint:        idgen.HashToProquint(dbEntry.ID),
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
			CollectionID:    dbEntry.CollectionID,
			ItemIndex:       dbEntry.ItemIndex,
			CollectionType:  dbEntry.CollectionType,
			ItemID:          dbEntry.ItemID,
		}
		listEntries = append(listEntries, listEntry)
	}

	// Calculate total pages
	totalPages := (queryResult.FilteredCount + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	// Create results
	results := &ListResults{
		Entries:      listEntries,
		TotalEntries: queryResult.FilteredCount, // Use filtered count as total for display
		TotalPages:   totalPages,
		CurrentPage:  page,
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



// FindEntry finds a specific entry by ID (full hash) or proquint
func (l *Lister) FindEntry(id string) (*ListEntry, error) {
	// First try direct lookup by full ID
	dbEntry, err := l.catalogDB.GetEntry(id)
	if err != nil {
		// If not found by full ID, try proquint lookup
		if errors.GetErrorCode(err) == errors.ErrCodeNotFound {
			dbEntry, err = l.catalogDB.GetEntryByProquint(id)
			if err != nil {
				if errors.GetErrorCode(err) == errors.ErrCodeNotFound {
					return nil, errors.NewInputError(
						fmt.Sprintf("Entry not found: %s", id),
						"Check the entry ID with 'pudl list'",
						"Ensure you're using the correct proquint identifier")
				}
				return nil, err
			}
		} else {
			return nil, err // Other database error
		}
	}

	// Convert database entry to list entry
	listEntry := &ListEntry{
		ID:              dbEntry.ID,
		Proquint:        idgen.HashToProquint(dbEntry.ID),
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
		CollectionID:    dbEntry.CollectionID,
		ItemIndex:       dbEntry.ItemIndex,
		CollectionType:  dbEntry.CollectionType,
		ItemID:          dbEntry.ItemID,
	}

	return listEntry, nil
}

// GetCollectionItems retrieves all items belonging to a collection
func (l *Lister) GetCollectionItems(collectionID string) ([]ListEntry, error) {
	dbItems, err := l.catalogDB.GetCollectionItems(collectionID)
	if err != nil {
		return nil, err
	}

	items := make([]ListEntry, len(dbItems))
	for i, dbEntry := range dbItems {
		items[i] = ListEntry{
			ID:              dbEntry.ID,
			Proquint:        idgen.HashToProquint(dbEntry.ID),
			StoredPath:      dbEntry.StoredPath,
			MetadataPath:    dbEntry.MetadataPath,
			ImportTimestamp: dbEntry.ImportTimestamp.Format("2006-01-02T15:04:05-07:00"),
			ParsedTimestamp: dbEntry.ImportTimestamp,
			Format:          dbEntry.Format,
			Origin:          dbEntry.Origin,
			Schema:          dbEntry.Schema,
			Confidence:      dbEntry.Confidence,
			RecordCount:     dbEntry.RecordCount,
			SizeBytes:       dbEntry.SizeBytes,
			CollectionID:    dbEntry.CollectionID,
			ItemIndex:       dbEntry.ItemIndex,
			CollectionType:  dbEntry.CollectionType,
			ItemID:          dbEntry.ItemID,
		}
	}

	return items, nil
}

// DeleteEntry deletes an entry and optionally its collection items
func (l *Lister) DeleteEntry(entryID string, cascade bool) (*DeleteResult, error) {
	// Get the entry first
	entry, err := l.catalogDB.GetEntry(entryID)
	if err != nil {
		return nil, err
	}

	result := &DeleteResult{
		Success:  true,
		EntryID:  entry.ID,
		Proquint: idgen.HashToProquint(entry.ID),
	}

	// If this is a collection and cascade is enabled, delete items first
	if entry.CollectionType != nil && *entry.CollectionType == "collection" && cascade {
		items, err := l.catalogDB.GetCollectionItems(entry.ID)
		if err != nil {
			return nil, errors.NewSystemError("Failed to get collection items", err)
		}

		for _, item := range items {
			// Delete item files
			_ = os.Remove(item.StoredPath)
			_ = os.Remove(item.MetadataPath)

			// Delete from database
			if err := l.catalogDB.DeleteEntry(item.ID); err != nil {
				return nil, errors.NewSystemError(fmt.Sprintf("Failed to delete item %s", item.ID), err)
			}

			result.DeletedItemIDs = append(result.DeletedItemIDs, idgen.HashToProquint(item.ID))
		}
		result.ItemsDeleted = len(items)
	}

	// Delete the entry's files
	if err := os.Remove(entry.StoredPath); err == nil {
		result.DataFileDeleted = true
	}
	if err := os.Remove(entry.MetadataPath); err == nil {
		result.MetadataFileDeleted = true
	}

	// Delete from database
	if err := l.catalogDB.DeleteEntry(entry.ID); err != nil {
		return nil, err
	}

	return result, nil
}
