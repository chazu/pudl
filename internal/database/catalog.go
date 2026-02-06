package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"pudl/internal/errors"
	"pudl/internal/idgen"
	"pudl/internal/schemaname"
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
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// FilterOptions contains filtering criteria for catalog queries
type FilterOptions struct {
	Schema         string // Filter by CUE schema
	Origin         string // Filter by data origin
	Format         string // Filter by file format
	CollectionID   string // Filter by collection ID
	CollectionType string // Filter by collection type ('collection', 'item')
	ItemID         string // Filter by item ID
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
	Entries      []CatalogEntry `json:"entries"`
	TotalCount   int            `json:"total_count"`
	FilteredCount int           `json:"filtered_count"`
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
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000")
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

// createTables creates the catalog table and indexes
func (c *CatalogDB) createTables() error {
	// Create catalog entries table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS catalog_entries (
		id TEXT PRIMARY KEY,
		stored_path TEXT NOT NULL,
		metadata_path TEXT NOT NULL,
		import_timestamp DATETIME NOT NULL,
		format TEXT NOT NULL,
		origin TEXT NOT NULL,
		schema TEXT NOT NULL,
		confidence REAL NOT NULL,
		record_count INTEGER NOT NULL,
		size_bytes INTEGER NOT NULL,
		-- Collection support fields
		collection_id TEXT,           -- Parent collection ID (NULL for non-collection items)
		item_index INTEGER,           -- Position in collection (NULL for collections and standalone items)
		collection_type TEXT,         -- 'collection', 'item', or NULL for standalone
		item_id TEXT,                 -- Unique identifier for collection items
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := c.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create catalog_entries table: %w", err)
	}

	// Create indexes for collection queries
	indexSQL := []string{
		`CREATE INDEX IF NOT EXISTS idx_collection_id ON catalog_entries(collection_id);`,
		`CREATE INDEX IF NOT EXISTS idx_collection_type ON catalog_entries(collection_type);`,
		`CREATE INDEX IF NOT EXISTS idx_item_index ON catalog_entries(collection_id, item_index);`,
		`CREATE INDEX IF NOT EXISTS idx_item_id ON catalog_entries(item_id);`,
	}

	for _, sql := range indexSQL {
		if _, err := c.db.Exec(sql); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Create indexes for common query patterns
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_catalog_schema ON catalog_entries(schema);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_origin ON catalog_entries(origin);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_format ON catalog_entries(format);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_import_timestamp ON catalog_entries(import_timestamp);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_size_bytes ON catalog_entries(size_bytes);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_record_count ON catalog_entries(record_count);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_confidence ON catalog_entries(confidence);",
		"CREATE INDEX IF NOT EXISTS idx_catalog_created_at ON catalog_entries(created_at);",
	}

	for _, indexSQL := range indexes {
		if _, err := c.db.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Run identity column migration (idempotent)
	if err := c.ensureIdentityColumns(); err != nil {
		return fmt.Errorf("failed to ensure identity columns: %w", err)
	}

	return nil
}

// Close closes the database connection
func (c *CatalogDB) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// AddEntry adds a new entry to the catalog
func (c *CatalogDB) AddEntry(entry CatalogEntry) error {
	// Normalize schema name to canonical format before storing
	entry.Schema = schemaname.Normalize(entry.Schema)

	insertSQL := `
	INSERT INTO catalog_entries (
		id, stored_path, metadata_path, import_timestamp, format, origin,
		schema, confidence, record_count, size_bytes, collection_id, item_index,
		collection_type, item_id, resource_id, content_hash, identity_json, version,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	entry.CreatedAt = now
	entry.UpdatedAt = now

	_, err := c.db.Exec(insertSQL,
		entry.ID, entry.StoredPath, entry.MetadataPath, entry.ImportTimestamp,
		entry.Format, entry.Origin, entry.Schema, entry.Confidence,
		entry.RecordCount, entry.SizeBytes, entry.CollectionID, entry.ItemIndex,
		entry.CollectionType, entry.ItemID, entry.ResourceID, entry.ContentHash,
		entry.IdentityJSON, entry.Version, entry.CreatedAt, entry.UpdatedAt)

	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to add catalog entry", err)
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
	selectSQL := `
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   created_at, updated_at
	FROM catalog_entries
	WHERE id = ?`

	var entry CatalogEntry
	err := c.db.QueryRow(selectSQL, id).Scan(
		&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
		&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
		&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
		&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
		&entry.IdentityJSON, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("Catalog entry not found: %s", id), nil)
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to retrieve catalog entry", err)
	}

	return &entry, nil
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
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   created_at, updated_at
	FROM catalog_entries
	WHERE id LIKE ?
	LIMIT 2`  // Limit 2 to detect ambiguous matches

	rows, err := c.db.Query(selectSQL, hexPrefix+"%")
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query by proquint", err)
	}
	defer rows.Close()

	var entries []CatalogEntry
	for rows.Next() {
		var entry CatalogEntry
		err := rows.Scan(
			&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
			&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
			&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
			&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
			&entry.IdentityJSON, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan entry", err)
		}
		entries = append(entries, entry)
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
		whereConditions = append(whereConditions, "collection_id = ?")
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
			"timestamp": "import_timestamp",
			"size":      "size_bytes",
			"records":   "record_count",
			"schema":    "schema",
			"origin":    "origin",
			"format":    "format",
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
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   created_at, updated_at
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
		var entry CatalogEntry
		err := rows.Scan(
			&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
			&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
			&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
			&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
			&entry.IdentityJSON, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan catalog entry", err)
		}
		entries = append(entries, entry)
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
	selectSQL := `
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   created_at, updated_at
	FROM catalog_entries
	WHERE collection_id = ? AND collection_type = 'item'
	ORDER BY item_index ASC`

	rows, err := c.db.Query(selectSQL, collectionID)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query collection items", err)
	}
	defer rows.Close()

	var items []CatalogEntry
	for rows.Next() {
		var entry CatalogEntry
		err := rows.Scan(
			&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
			&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
			&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
			&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
			&entry.IdentityJSON, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan collection item", err)
		}
		items = append(items, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Error iterating collection items", err)
	}

	return items, nil
}

// GetCollectionByID retrieves a collection entry by ID
func (c *CatalogDB) GetCollectionByID(collectionID string) (*CatalogEntry, error) {
	selectSQL := `
	SELECT id, stored_path, metadata_path, import_timestamp, format, origin,
		   schema, confidence, record_count, size_bytes, collection_id, item_index,
		   collection_type, item_id, resource_id, content_hash, identity_json, version,
		   created_at, updated_at
	FROM catalog_entries
	WHERE id = ? AND collection_type = 'collection'`

	var entry CatalogEntry
	err := c.db.QueryRow(selectSQL, collectionID).Scan(
		&entry.ID, &entry.StoredPath, &entry.MetadataPath, &entry.ImportTimestamp,
		&entry.Format, &entry.Origin, &entry.Schema, &entry.Confidence,
		&entry.RecordCount, &entry.SizeBytes, &entry.CollectionID, &entry.ItemIndex,
		&entry.CollectionType, &entry.ItemID, &entry.ResourceID, &entry.ContentHash,
		&entry.IdentityJSON, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, errors.WrapError(errors.ErrCodeNotFound, fmt.Sprintf("Collection not found: %s", collectionID), nil)
	}
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to retrieve collection", err)
	}

	return &entry, nil
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
		updated_at = ?
	WHERE id = ?`

	entry.UpdatedAt = time.Now()

	result, err := c.db.Exec(updateSQL,
		entry.StoredPath, entry.MetadataPath, entry.ImportTimestamp, entry.Format,
		entry.Origin, entry.Schema, entry.Confidence, entry.RecordCount, entry.SizeBytes,
		entry.ResourceID, entry.ContentHash, entry.IdentityJSON, entry.Version,
		entry.UpdatedAt, entry.ID)

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
