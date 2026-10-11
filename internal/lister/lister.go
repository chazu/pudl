package lister

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/idgen"
)

// Lister handles data listing and querying operations
type Lister struct {
	dataPath  string
	catalogDB *database.CatalogDB
}

// FilterOptions contains filtering criteria for listing data
type FilterOptions struct {
	Match          func(database.CatalogEntry) (bool, error) // optional payload predicate
	Schema         string                                    // Filter by CUE schema
	Origin         string                                    // Filter by data origin
	Format         string                                    // Filter by file format
	CollectionID   string                                    // Filter by collection ID
	CollectionType string                                    // Filter by collection type ('collection', 'item')
	ItemID         string                                    // Filter by item ID
	EntryTypes     []string                                  // Filter by entry type (e.g. "observe", "manifest", "manifest-action"); empty = no filter
}

// DisplayOptions contains display preferences for listing data
type DisplayOptions struct {
	All     bool
	Context context.Context
	Verbose bool   // Show detailed information
	Limit   int    // Maximum number of results
	SortBy  string // Field to sort by
	Reverse bool   // Reverse sort order
	Page    int    // Page number (1-based)
	PerPage int    // Results per page
}

// ListEntry represents a single entry in the list results
type ListEntry struct {
	Fields          map[string]any `json:"fields,omitempty"`
	MissingFields   []string       `json:"missing_fields,omitempty"`
	ID              string         `json:"id"`
	Proquint        string         `json:"proquint"` // Human-friendly ID derived from content hash
	StoredPath      string         `json:"stored_path"`
	MetadataPath    string         `json:"metadata_path"`
	ImportTimestamp string         `json:"import_timestamp"`
	ParsedTimestamp time.Time      `json:"-"` // For sorting
	Format          string         `json:"format"`
	Origin          string         `json:"origin"`
	Schema          string         `json:"schema"`
	Confidence      float64        `json:"confidence"`
	RecordCount     int            `json:"record_count"`
	SizeBytes       int64          `json:"size_bytes"`
	// Collection fields
	CollectionID   *string `json:"collection_id,omitempty"`
	ItemIndex      *int    `json:"item_index,omitempty"`
	CollectionType *string `json:"collection_type,omitempty"`
	ItemID         *string `json:"item_id,omitempty"`
	// Identity tracking fields
	ResourceID   *string `json:"resource_id,omitempty"`
	ContentHash  *string `json:"content_hash,omitempty"`
	IdentityJSON *string `json:"identity_json,omitempty"`
	Version      *int    `json:"version,omitempty"`
	// Artifact tracking fields
	EntryType *string `json:"entry_type,omitempty"`
	Target    *string `json:"target,omitempty"`
	RunID     *string `json:"run_id,omitempty"`
	Tags      *string `json:"tags,omitempty"`
}

// ListResults contains the results of a list operation
type ListResults struct {
	Entries       []ListEntry `json:"entries"`
	TotalEntries  int         `json:"total_entries"` // listable entries: matches capped by --limit
	TotalMatched  int         `json:"total_matched"` // entries matching the filters, before --limit
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
	CleanupErrors       []string `json:"cleanup_errors,omitempty"`
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
	// dataPath is <active-pudl-root>/data, so its parent owns the catalog.
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
	pageSize := limitedPageSize(displayOpts.Limit, offset, perPage)

	// Convert display options to database query options. A page entirely past
	// the limit still queries one page so the filtered count is available.
	queryOpts := database.QueryOptions{
		Limit:   perPage,
		Offset:  offset,
		SortBy:  displayOpts.SortBy,
		Reverse: displayOpts.Reverse,
	}
	if displayOpts.All || filters.Match != nil {
		queryOpts.Limit = 0
		queryOpts.Offset = 0
	}

	// Convert filters to database filters
	dbFilters := database.FilterOptions{
		Schema:         filters.Schema,
		Origin:         filters.Origin,
		Format:         filters.Format,
		CollectionID:   filters.CollectionID,
		CollectionType: filters.CollectionType,
		ItemID:         filters.ItemID,
		EntryTypes:     filters.EntryTypes,
	}

	// Query database
	ctx := displayOpts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	queryResult, err := l.catalogDB.QueryEntriesContext(ctx, dbFilters, queryOpts)
	if err != nil {
		return nil, err // Already a PUDLError from database
	}

	// Convert database entries to list entries, stopping at the limit
	entries := queryResult.Entries
	if filters.Match != nil {
		matched := make([]database.CatalogEntry, 0)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			ok, err := filters.Match(entry)
			if err != nil {
				return nil, err
			}
			if ok {
				matched = append(matched, entry)
			}
		}
		entries = matched
		queryResult.FilteredCount = len(entries)
	}
	if displayOpts.All {
		page = 1
		offset = 0
		perPage = len(entries)
		if perPage == 0 {
			perPage = 1
		}
		pageSize = limitedPageSize(displayOpts.Limit, 0, perPage)
	} else if filters.Match != nil {
		if offset >= len(entries) {
			entries = nil
		} else {
			entries = entries[offset:]
		}
	}
	if len(entries) > pageSize {
		entries = entries[:pageSize]
	}
	var listEntries []ListEntry
	for _, dbEntry := range entries {
		listEntry := dbEntryToListEntry(dbEntry)
		listEntries = append(listEntries, listEntry)
	}

	// The listed total is the filtered count, capped by the limit.
	totalEntries := queryResult.FilteredCount
	if displayOpts.Limit > 0 && displayOpts.Limit < totalEntries {
		totalEntries = displayOpts.Limit
	}

	// Calculate total pages
	totalPages := (totalEntries + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	// Create results
	results := &ListResults{
		Entries:      listEntries,
		TotalEntries: totalEntries,
		TotalMatched: queryResult.FilteredCount,
		TotalPages:   totalPages,
		CurrentPage:  page,
	}

	// Calculate summary statistics
	l.calculateSummaryStats(results, listEntries)

	return results, nil
}

// limitedPageSize is how many entries of the page starting at offset fall
// within limit (0 means no limit).
func limitedPageSize(limit, offset, perPage int) int {
	if limit <= 0 {
		return perPage
	}
	remaining := limit - offset
	if remaining <= 0 {
		return 0
	}
	if remaining < perPage {
		return remaining
	}
	return perPage
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

	le := dbEntryToListEntry(*dbEntry)
	return &le, nil
}

// GetCollectionItems retrieves all items belonging to a collection
func (l *Lister) GetCollectionItems(collectionID string) ([]ListEntry, error) {
	dbItems, err := l.catalogDB.GetCollectionItems(collectionID)
	if err != nil {
		return nil, err
	}

	items := make([]ListEntry, len(dbItems))
	for i, dbEntry := range dbItems {
		items[i] = dbEntryToListEntry(dbEntry)
	}

	return items, nil
}

// DeleteEntry deletes an entry and optionally its collection items
func (l *Lister) DeleteEntry(entryID string, cascade bool) (*DeleteResult, error) {
	return l.DeleteEntryContext(context.Background(), entryID, cascade)
}

func (l *Lister) DeleteEntryContext(ctx context.Context, entryID string, cascade bool) (*DeleteResult, error) {
	var result *DeleteResult
	err := artifacts.WithLock(ctx, l.catalogDB.Root(), func() error {
		plan, err := l.catalogDB.DeleteEntriesAtomicContext(ctx, entryID, cascade)
		if err != nil {
			return err
		}
		result = &DeleteResult{Success: true, EntryID: plan.Entry.ID, Proquint: idgen.HashToProquint(plan.Entry.ID), ItemsDeleted: len(plan.DeletedItems)}
		cleanup := func(path string) bool {
			removed, err := l.catalogDB.RemoveCommittedOrphan(path)
			if err != nil {
				result.CleanupErrors = append(result.CleanupErrors, err.Error())
			}
			return removed
		}
		result.DataFileDeleted = cleanup(plan.Entry.StoredPath)
		result.MetadataFileDeleted = cleanup(plan.Entry.MetadataPath)
		for _, item := range plan.DeletedItems {
			cleanup(item.StoredPath)
			cleanup(item.MetadataPath)
			result.DeletedItemIDs = append(result.DeletedItemIDs, idgen.HashToProquint(item.ID))
		}
		return nil
	})
	return result, err
}

// dbEntryToListEntry converts a database CatalogEntry to a ListEntry.
func dbEntryToListEntry(e database.CatalogEntry) ListEntry {
	return ListEntry{
		ID:              e.ID,
		Proquint:        idgen.HashToProquint(e.ID),
		StoredPath:      e.StoredPath,
		MetadataPath:    e.MetadataPath,
		ImportTimestamp: e.ImportTimestamp.Format(time.RFC3339),
		ParsedTimestamp: e.ImportTimestamp,
		Format:          e.Format,
		Origin:          e.Origin,
		Schema:          e.Schema,
		Confidence:      e.Confidence,
		RecordCount:     e.RecordCount,
		SizeBytes:       e.SizeBytes,
		CollectionID:    e.CollectionID,
		ItemIndex:       e.ItemIndex,
		CollectionType:  e.CollectionType,
		ItemID:          e.ItemID,
		ResourceID:      e.ResourceID,
		ContentHash:     e.ContentHash,
		IdentityJSON:    e.IdentityJSON,
		Version:         e.Version,
		EntryType:       e.EntryType,
		Target:          e.Target,
		RunID:           e.RunID,
		Tags:            e.Tags,
	}
}
