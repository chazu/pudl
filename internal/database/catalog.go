package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/schemaname"
	_ "modernc.org/sqlite"
)

// CatalogDB handles SQLite database operations for the catalog
type CatalogDB struct {
	db        *sql.DB
	configDir string
}

// CatalogEntry represents an entry in the catalog database
type CatalogEntry struct {
	ID              string    `json:"id"`
	StoredPath      string    `json:"stored_path"`
	MetadataPath    string    `json:"metadata_path"`
	ImportTimestamp time.Time `json:"import_timestamp"`
	Format          string    `json:"format"`
	Origin          string    `json:"origin"`
	Schema          string    `json:"schema"`
	Confidence      float64   `json:"confidence"`
	RecordCount     int       `json:"record_count"`
	SizeBytes       int64     `json:"size_bytes"`
	// Collection support fields
	CollectionID   *string `json:"collection_id,omitempty"`   // Parent collection ID
	ItemIndex      *int    `json:"item_index,omitempty"`      // Position in collection
	CollectionType *string `json:"collection_type,omitempty"` // 'collection', 'item', or nil
	ItemID         *string `json:"item_id,omitempty"`         // Unique identifier for items
	// Identity tracking fields
	ResourceID   *string `json:"resource_id,omitempty"`   // Deterministic hash of (schema, identity)
	ContentHash  *string `json:"content_hash,omitempty"`  // SHA256 of raw stored data
	IdentityJSON *string `json:"identity_json,omitempty"` // Canonical JSON of identity field values
	Version      *int    `json:"version,omitempty"`       // Monotonic version per resource_id
	// Artifact tracking fields
	EntryType *string `json:"entry_type,omitempty"` // e.g. "observe", "manifest", "manifest-action"
	Target    *string `json:"target,omitempty"`     // mu target / run target name (e.g. //models/<name>, home/odroid)
	RunID     *string `json:"run_id,omitempty"`     // Unique run identifier
	Tags      *string `json:"tags,omitempty"`       // JSON-encoded map[string]string
	// Convergence status tracking
	Status    *string   `json:"status,omitempty"` // Convergence status (unknown/clean/drifted/converging/failed)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FilterOptions contains filtering criteria for catalog queries
type FilterOptions struct {
	Schema         string   // Filter by CUE schema
	Origin         string   // Filter by data origin
	Format         string   // Filter by file format
	CollectionID   string   // Filter by collection ID
	CollectionType string   // Filter by collection type ('collection', 'item')
	ItemID         string   // Filter by item ID
	EntryTypes     []string // Filter by entry type (e.g. "observe", "manifest", "manifest-action"); empty = no filter
}

// QueryOptions contains query configuration
type QueryOptions struct {
	Limit   int    // Maximum number of results (0 = no limit)
	Offset  int    // Number of results to skip
	SortBy  string // Field to sort by
	Reverse bool   // Reverse sort order
}

// QueryResult contains the results of a catalog query
type QueryResult struct {
	Entries       []CatalogEntry `json:"entries"`
	TotalCount    int            `json:"total_count"`
	FilteredCount int            `json:"filtered_count"`
}

// NewCatalogDB creates a new catalog database instance
// configDir should be the PUDL config directory (e.g., ~/.pudl)
func NewCatalogDB(configDir string) (*CatalogDB, error) {
	db := &CatalogDB{
		configDir: configDir,
	}

	if err := db.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize catalog database: %w", err)
	}

	return db, nil
}

// initialize sets up the database connection and creates tables if needed
func (c *CatalogDB) initialize() error {
	// Ensure sqlite directory exists under config/data/sqlite/
	sqliteDir := filepath.Join(c.configDir, "data", "sqlite")
	if err := os.MkdirAll(sqliteDir, 0755); err != nil {
		return fmt.Errorf("failed to create sqlite directory: %w", err)
	}

	// Open database connection
	dbPath := filepath.Join(sqliteDir, "catalog.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(10000)")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	c.db = db

	// Create tables and indexes
	if err := c.createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// createTables brings the catalog schema up to date and (re)builds the views.
//
// Schema changes run as ordered, recorded migrations — see migrations.go. Views
// and syncs deliberately run every open: a view is a restatement of the Go source
// that declares it, so versioning it would make a change to its body a no-op
// until someone bumped a number.
func (c *CatalogDB) createTables() error {
	if err := c.runMigrations(); err != nil {
		return err
	}

	// Sync, not schema: guarded by its own emptiness check.
	if err := c.backfillCurrentFacts(); err != nil {
		return fmt.Errorf("failed to backfill current_facts: %w", err)
	}

	// Create the catalog_entry_edb view exposing catalog_entries to Datalog.
	// Must run after the migrations: it references migration-added columns.
	if err := c.ensureCatalogEntryView(); err != nil {
		return fmt.Errorf("failed to ensure catalog_entry view: %w", err)
	}

	// Create the fact_scored_edb view exposing current facts with read-time decay
	// scoring to Datalog. Must run after the facts and current_facts tables exist.
	if err := c.ensureFactScoredView(); err != nil {
		return fmt.Errorf("failed to ensure fact_scored view: %w", err)
	}

	return nil
}

// DB returns the underlying *sql.DB for direct query execution.
func (c *CatalogDB) DB() *sql.DB {
	return c.db
}

// Close closes the database connection
func (c *CatalogDB) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// catalogTimeLayout is the canonical text layout for DATETIME columns. Binding
// a raw time.Time lets the modernc/sqlite driver store Go's Time.String() output
// (e.g. "... -0400 -0400" for a fixed-offset/parsed time, or "... MDT m=+..."
// for a monotonic-clock time), which the driver cannot scan back into time.Time.
// Formatting to this layout (the format existing rows already use) keeps
// timestamps round-trippable.
const catalogTimeLayout = "2006-01-02 15:04:05.999999999-07:00"

// formatCatalogTime renders a timestamp for storage in a DATETIME column in a
// form the driver can scan back into time.Time.
func formatCatalogTime(t time.Time) string {
	return t.Format(catalogTimeLayout)
}

// AddEntry adds a new entry to the catalog.
func (c *CatalogDB) AddEntry(entry CatalogEntry) error {
	return addEntryIn(c.db, entry)
}

// addEntryIn is the executor-parameterized form of AddEntry: the same insert,
// run either standalone or inside a CatalogTx. When the entry is a collection
// item its membership row is written too, and inside a transaction the pair
// commits or rolls back together.
func addEntryIn(q dbtx, entry CatalogEntry) error {
	// Normalize schema name to canonical format before storing
	entry.Schema = schemaname.Normalize(entry.Schema)

	// collection_id and item_index are absent: an entry's collection membership is
	// recorded in collection_memberships below, which is the only relationship
	// source. Entry.CollectionID is still read on the way in — it means "record a
	// membership in this collection" — it just no longer becomes a column that can
	// disagree with the table.
	insertSQL := `
	INSERT INTO catalog_entries (
		id, stored_path, metadata_path, import_timestamp, format, origin,
		schema, confidence, record_count, size_bytes,
		collection_type, item_id, resource_id, content_hash, identity_json, version,
		entry_type, target, run_id, tags, status,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	entry.CreatedAt = now
	entry.UpdatedAt = now

	// Default status to "unknown" if not set
	if entry.Status == nil {
		defaultStatus := "unknown"
		entry.Status = &defaultStatus
	}

	// Defaults applied at insert, not repaired later.
	//
	// These used to be filled in by backfillDefaults, which ran on every open —
	// so a row written without them stayed incomplete until the *next* process
	// opened the catalog. That worked only because every migration re-ran every
	// time; under recorded migrations a backfill runs once, and a row written
	// after it would never be repaired. Setting the value where the row is
	// created is the right place for it either way: the end state is identical,
	// it just arrives immediately instead of after a restart.
	if entry.ContentHash == nil {
		contentHash := entry.ID
		entry.ContentHash = &contentHash
	}
	if entry.Version == nil {
		version := 1
		entry.Version = &version
	}

	_, err := q.Exec(insertSQL,
		entry.ID, entry.StoredPath, entry.MetadataPath, formatCatalogTime(entry.ImportTimestamp),
		entry.Format, entry.Origin, entry.Schema, entry.Confidence,
		entry.RecordCount, entry.SizeBytes,
		entry.CollectionType, entry.ItemID, entry.ResourceID, entry.ContentHash,
		entry.IdentityJSON, entry.Version, entry.EntryType, entry.Target,
		entry.RunID, entry.Tags, entry.Status, formatCatalogTime(entry.CreatedAt), formatCatalogTime(entry.UpdatedAt))

	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to add catalog entry", err)
	}
	if entry.CollectionType != nil && *entry.CollectionType == "item" && entry.CollectionID != nil {
		itemIndex := 0
		if entry.ItemIndex != nil {
			itemIndex = *entry.ItemIndex
		}
		if err := addCollectionMembershipIn(q, *entry.CollectionID, entry.ID, itemIndex); err != nil {
			return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to add collection membership", err)
		}
	}

	return nil
}

// EntryExists checks if a catalog entry with the given ID exists
func (c *CatalogDB) EntryExists(id string) (bool, error) {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM catalog_entries WHERE id = ?", id).Scan(&count)
	if err != nil {
		return false, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to check entry existence", err)
	}
	return count > 0, nil
}

// GetEntry retrieves a specific entry by ID
func (c *CatalogDB) GetEntry(id string) (*CatalogEntry, error) {
	return getEntryIn(c.db, id)
}

// getEntryIn is the executor-parameterized form of GetEntry.
func getEntryIn(q dbtx, id string) (*CatalogEntry, error) {
	entry, err := scanEntry(q.QueryRow(
		`SELECT `+entrySelect("catalog_entries")+` FROM catalog_entries WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("Catalog entry not found: %s", id), nil)
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to retrieve catalog entry", err)
	}
	return entry, nil
}

// GetEntryByProquint retrieves an entry by its proquint identifier
// Proquints are derived from the first 32 bits of the content hash
func (c *CatalogDB) GetEntryByProquint(proquint string) (*CatalogEntry, error) {
	// Convert proquint to the hex prefix it represents
	num, err := idgen.ProquintToNumber(proquint)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, fmt.Sprintf("Invalid proquint: %s", proquint), err)
	}

	// Convert to 8-character hex prefix
	hexPrefix := idgen.Uint32ToHash(num)

	// Query for entries where ID starts with this prefix
	selectSQL := `
	SELECT ` + entrySelect("catalog_entries") + `
	FROM catalog_entries
	WHERE id LIKE ?
	LIMIT 2` // Limit 2 to detect ambiguous matches

	rows, err := c.db.Query(selectSQL, hexPrefix+"%")
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query by proquint", err)
	}
	defer rows.Close()

	var entries []CatalogEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan entry", err)
		}
		entries = append(entries, *entry)
	}

	if len(entries) == 0 {
		return nil, errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("No entry found for proquint: %s", proquint), nil)
	}

	if len(entries) > 1 {
		// Multiple entries share this proquint prefix (hash collision on first 32 bits)
		// Return an error with guidance
		return nil, errors.NewInputError(
			fmt.Sprintf("Ambiguous proquint: %s matches multiple entries", proquint),
			"Use the full hash ID to specify the exact entry",
			fmt.Sprintf("Matching IDs: %s, %s", entries[0].ID[:16]+"...", entries[1].ID[:16]+"..."))
	}

	return &entries[0], nil
}

// QueryEntries queries catalog entries with filtering, sorting, and pagination
func (c *CatalogDB) QueryEntries(filters FilterOptions, options QueryOptions) (*QueryResult, error) {
	// Build WHERE clause
	var whereConditions []string
	var args []interface{}

	if filters.Schema != "" {
		whereConditions = append(whereConditions, "schema LIKE ?")
		args = append(args, "%"+filters.Schema+"%")
	}
	if filters.Origin != "" {
		whereConditions = append(whereConditions, "origin LIKE ?")
		args = append(args, "%"+filters.Origin+"%")
	}
	if filters.Format != "" {
		whereConditions = append(whereConditions, "format LIKE ?")
		args = append(args, "%"+filters.Format+"%")
	}
	if filters.CollectionID != "" {
		whereConditions = append(whereConditions, "id IN (SELECT item_id FROM collection_memberships WHERE collection_id = ?)")
		args = append(args, filters.CollectionID)
	}
	if filters.CollectionType != "" {
		whereConditions = append(whereConditions, "collection_type = ?")
		args = append(args, filters.CollectionType)
	}
	if filters.ItemID != "" {
		whereConditions = append(whereConditions, "item_id = ?")
		args = append(args, filters.ItemID)
	}
	if len(filters.EntryTypes) > 0 {
		placeholders := make([]string, len(filters.EntryTypes))
		for i, t := range filters.EntryTypes {
			placeholders[i] = "?"
			args = append(args, t)
		}
		whereConditions = append(whereConditions, "entry_type IN ("+strings.Join(placeholders, ",")+")")
	}

	whereClause := ""
	if len(whereConditions) > 0 {
		whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
	}

	// Get total count (without filters)
	var totalCount int
	err := c.db.QueryRow("SELECT COUNT(*) FROM catalog_entries").Scan(&totalCount)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get total count", err)
	}

	// Get filtered count
	var filteredCount int
	countSQL := "SELECT COUNT(*) FROM catalog_entries " + whereClause
	err = c.db.QueryRow(countSQL, args...).Scan(&filteredCount)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get filtered count", err)
	}

	// Build ORDER BY clause
	orderBy := "import_timestamp DESC" // Default sort
	if options.SortBy != "" {
		validSortFields := map[string]string{
			"timestamp":  "import_timestamp",
			"size":       "size_bytes",
			"records":    "record_count",
			"schema":     "schema",
			"origin":     "origin",
			"format":     "format",
			"confidence": "confidence",
		}

		if dbField, valid := validSortFields[options.SortBy]; valid {
			direction := "ASC"
			if options.Reverse {
				direction = "DESC"
			}
			orderBy = fmt.Sprintf("%s %s", dbField, direction)
		}
	}

	// Build main query with LIMIT and OFFSET
	selectSQL := fmt.Sprintf(`
	SELECT `+entrySelect("catalog_entries")+`
	FROM catalog_entries
	%s
	ORDER BY %s`, whereClause, orderBy)

	if options.Limit > 0 {
		selectSQL += fmt.Sprintf(" LIMIT %d", options.Limit)
	}
	if options.Offset > 0 {
		selectSQL += fmt.Sprintf(" OFFSET %d", options.Offset)
	}

	// Execute query
	rows, err := c.db.Query(selectSQL, args...)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query catalog entries", err)
	}
	defer rows.Close()

	// Scan results
	var entries []CatalogEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan catalog entry", err)
		}
		entries = append(entries, *entry)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Error iterating catalog entries", err)
	}

	return &QueryResult{
		Entries:       entries,
		TotalCount:    totalCount,
		FilteredCount: filteredCount,
	}, nil
}

// GetUniqueValues returns unique values for a specific field
func (c *CatalogDB) GetUniqueValues(field string) ([]string, error) {
	validFields := map[string]string{
		"schema": "schema",
		"origin": "origin",
		"format": "format",
	}

	dbField, valid := validFields[field]
	if !valid {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, fmt.Sprintf("Invalid field for unique values: %s", field), nil)
	}

	selectSQL := fmt.Sprintf("SELECT DISTINCT %s FROM catalog_entries ORDER BY %s", dbField, dbField)

	rows, err := c.db.Query(selectSQL)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query unique values", err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan unique value", err)
		}
		values = append(values, value)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Error iterating unique values", err)
	}

	return values, nil
}

// GetDistinctOrigins returns a list of distinct origins from the catalog
func (c *CatalogDB) GetDistinctOrigins() ([]string, error) {
	selectSQL := "SELECT DISTINCT origin FROM catalog_entries WHERE origin IS NOT NULL AND origin != '' ORDER BY origin"

	rows, err := c.db.Query(selectSQL)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query distinct origins", err)
	}
	defer rows.Close()

	var origins []string
	for rows.Next() {
		var origin string
		if err := rows.Scan(&origin); err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan origin", err)
		}
		origins = append(origins, origin)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Error iterating origins", err)
	}

	return origins, nil
}

// GetCollectionItems retrieves all items belonging to a collection
func (c *CatalogDB) GetCollectionItems(collectionID string) ([]CatalogEntry, error) {
	// Read through a specific membership: these entries carry *that* collection
	// and their index within it, which is the only well-defined answer for an
	// item shared between collections.
	selectSQL := `
	SELECT ` + entrySelectVia("ce", "cm") + `
	FROM collection_memberships cm
	JOIN catalog_entries ce ON ce.id = cm.item_id
	WHERE cm.collection_id = ?
	ORDER BY cm.item_index ASC`

	rows, err := c.db.Query(selectSQL, collectionID)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query collection items", err)
	}
	defer rows.Close()

	var items []CatalogEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan collection item", err)
		}
		items = append(items, *entry)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Error iterating collection items", err)
	}

	return items, nil
}

// GetCollectionByID retrieves a collection entry by ID
func (c *CatalogDB) GetCollectionByID(collectionID string) (*CatalogEntry, error) {
	selectSQL := `
	SELECT ` + entrySelect("catalog_entries") + `
	FROM catalog_entries
	WHERE id = ? AND collection_type = 'collection'`

	entry, err := scanEntry(c.db.QueryRow(selectSQL, collectionID))
	if err == sql.ErrNoRows {
		return nil, errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("Collection not found: %s", collectionID), nil)
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to retrieve collection", err)
	}

	return entry, nil
}

// UpdateEntry updates an existing catalog entry
func (c *CatalogDB) UpdateEntry(entry CatalogEntry) error {
	// Normalize schema name to canonical format before storing
	entry.Schema = schemaname.Normalize(entry.Schema)

	updateSQL := `
	UPDATE catalog_entries SET
		stored_path = ?, metadata_path = ?, import_timestamp = ?, format = ?,
		origin = ?, schema = ?, confidence = ?, record_count = ?, size_bytes = ?,
		resource_id = ?, content_hash = ?, identity_json = ?, version = ?,
		entry_type = ?, target = ?, run_id = ?, tags = ?,
		updated_at = ?
	WHERE id = ?`

	entry.UpdatedAt = time.Now()

	result, err := c.db.Exec(updateSQL,
		entry.StoredPath, entry.MetadataPath, formatCatalogTime(entry.ImportTimestamp), entry.Format,
		entry.Origin, entry.Schema, entry.Confidence, entry.RecordCount, entry.SizeBytes,
		entry.ResourceID, entry.ContentHash, entry.IdentityJSON, entry.Version,
		entry.EntryType, entry.Target, entry.RunID, entry.Tags,
		formatCatalogTime(entry.UpdatedAt), entry.ID)

	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to update catalog entry", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get rows affected", err)
	}

	if rowsAffected == 0 {
		return errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("Catalog entry not found: %s", entry.ID), nil)
	}

	return nil
}

// DeleteEntry removes a catalog entry by ID
func (c *CatalogDB) DeleteEntry(id string) error {
	var collectionType string
	err := c.db.QueryRow("SELECT COALESCE(collection_type, '') FROM catalog_entries WHERE id = ?", id).Scan(&collectionType)
	if err == sql.ErrNoRows {
		return errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("Catalog entry not found: %s", id), nil)
	}
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to inspect catalog entry", err)
	}
	if collectionType == "collection" {
		if err := c.RemoveCollectionMemberships(id); err != nil {
			return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to delete collection memberships", err)
		}
	} else {
		if _, err := c.db.Exec("DELETE FROM collection_memberships WHERE item_id = ?", id); err != nil {
			return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to delete item memberships", err)
		}
	}
	deleteSQL := "DELETE FROM catalog_entries WHERE id = ?"

	result, err := c.db.Exec(deleteSQL, id)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to delete catalog entry", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get rows affected", err)
	}

	if rowsAffected == 0 {
		return errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("Catalog entry not found: %s", id), nil)
	}

	return nil
}

// MigrateSchemaNames normalizes all existing schema names in the database to canonical format.
// Returns the number of entries updated.
func (c *CatalogDB) MigrateSchemaNames() (int, error) {
	// Get all entries with their current schema names
	rows, err := c.db.Query("SELECT id, schema FROM catalog_entries")
	if err != nil {
		return 0, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query catalog entries", err)
	}
	defer rows.Close()

	// Collect entries that need updating
	type update struct {
		id        string
		newSchema string
	}
	var updates []update

	for rows.Next() {
		var id, schema string
		if err := rows.Scan(&id, &schema); err != nil {
			return 0, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan row", err)
		}

		normalized := schemaname.Normalize(schema)
		if normalized != schema {
			updates = append(updates, update{id: id, newSchema: normalized})
		}
	}

	if err := rows.Err(); err != nil {
		return 0, errors.WrapError(errors.ErrCodeDatabaseError, "Error iterating rows", err)
	}

	// Update entries that need normalization
	for _, u := range updates {
		_, err := c.db.Exec("UPDATE catalog_entries SET schema = ?, updated_at = ? WHERE id = ?",
			u.newSchema, time.Now(), u.id)
		if err != nil {
			return 0, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to update entry schema", err)
		}
	}

	return len(updates), nil
}
