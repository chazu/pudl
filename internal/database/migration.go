package database

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pudl/internal/errors"
)

// JSONCatalogEntry represents the old JSON catalog entry format
type JSONCatalogEntry struct {
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

// JSONCatalog represents the old JSON catalog format
type JSONCatalog struct {
	Entries     []JSONCatalogEntry `json:"entries"`
	LastUpdated string             `json:"last_updated"`
	Version     string             `json:"version"`
}

// MigrationResult contains the results of a catalog migration
type MigrationResult struct {
	TotalEntries    int      `json:"total_entries"`
	MigratedEntries int      `json:"migrated_entries"`
	SkippedEntries  int      `json:"skipped_entries"`
	Errors          []string `json:"errors"`
	BackupPath      string   `json:"backup_path"`
}

// MigrateFromJSON migrates catalog data from JSON format to SQLite
func (c *CatalogDB) MigrateFromJSON() (*MigrationResult, error) {
	result := &MigrationResult{
		Errors: []string{},
	}
	
	// Check if JSON catalog exists
	jsonCatalogPath := filepath.Join(c.dataPath, "catalog", "inventory.json")
	if _, err := os.Stat(jsonCatalogPath); os.IsNotExist(err) {
		// No JSON catalog to migrate
		return result, nil
	}
	
	// Load JSON catalog
	jsonCatalog, err := c.loadJSONCatalog(jsonCatalogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load JSON catalog: %w", err)
	}
	
	result.TotalEntries = len(jsonCatalog.Entries)
	
	// Create backup of JSON catalog
	backupPath, err := c.createBackup(jsonCatalogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}
	result.BackupPath = backupPath
	
	// Begin transaction for migration
	tx, err := c.db.Begin()
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to begin migration transaction", err)
	}
	defer tx.Rollback() // Will be ignored if tx.Commit() succeeds
	
	// Migrate each entry
	for _, jsonEntry := range jsonCatalog.Entries {
		entry, err := c.convertJSONEntry(jsonEntry)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to convert entry %s: %v", jsonEntry.ID, err))
			result.SkippedEntries++
			continue
		}
		
		// Insert entry using transaction
		insertSQL := `
		INSERT OR REPLACE INTO catalog_entries (
			id, stored_path, metadata_path, import_timestamp, format, origin, 
			schema, confidence, record_count, size_bytes, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		
		_, err = tx.Exec(insertSQL,
			entry.ID, entry.StoredPath, entry.MetadataPath, entry.ImportTimestamp,
			entry.Format, entry.Origin, entry.Schema, entry.Confidence,
			entry.RecordCount, entry.SizeBytes, entry.CreatedAt, entry.UpdatedAt)
		
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to insert entry %s: %v", entry.ID, err))
			result.SkippedEntries++
			continue
		}
		
		result.MigratedEntries++
	}
	
	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to commit migration transaction", err)
	}
	
	// If migration was successful, rename JSON catalog to indicate it's been migrated
	migratedPath := jsonCatalogPath + ".migrated"
	if err := os.Rename(jsonCatalogPath, migratedPath); err != nil {
		// Log warning but don't fail the migration
		result.Errors = append(result.Errors, fmt.Sprintf("Warning: Failed to rename JSON catalog: %v", err))
	}
	
	return result, nil
}

// loadJSONCatalog loads the JSON catalog from disk
func (c *CatalogDB) loadJSONCatalog(path string) (*JSONCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeFileSystem, "Failed to read JSON catalog file", err)
	}
	
	var catalog JSONCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, errors.WrapError(errors.ErrCodeParsingFailed, "Failed to parse JSON catalog", err)
	}
	
	return &catalog, nil
}

// convertJSONEntry converts a JSON catalog entry to SQLite format
func (c *CatalogDB) convertJSONEntry(jsonEntry JSONCatalogEntry) (*CatalogEntry, error) {
	// Parse timestamp
	importTime, err := time.Parse(time.RFC3339, jsonEntry.ImportTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse import timestamp: %w", err)
	}
	
	now := time.Now()
	
	entry := &CatalogEntry{
		ID:              jsonEntry.ID,
		StoredPath:      jsonEntry.StoredPath,
		MetadataPath:    jsonEntry.MetadataPath,
		ImportTimestamp: importTime,
		Format:          jsonEntry.Format,
		Origin:          jsonEntry.Origin,
		Schema:          jsonEntry.Schema,
		Confidence:      jsonEntry.Confidence,
		RecordCount:     jsonEntry.RecordCount,
		SizeBytes:       jsonEntry.SizeBytes,
		CreatedAt:       now, // Use current time for migration
		UpdatedAt:       now,
	}
	
	return entry, nil
}

// createBackup creates a backup of the JSON catalog file
func (c *CatalogDB) createBackup(jsonPath string) (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.backup_%s", jsonPath, timestamp)
	
	// Copy file
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return "", fmt.Errorf("failed to read original file: %w", err)
	}
	
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}
	
	return backupPath, nil
}

// CheckMigrationNeeded checks if migration from JSON to SQLite is needed
func (c *CatalogDB) CheckMigrationNeeded() (bool, error) {
	// Check if JSON catalog exists
	jsonCatalogPath := filepath.Join(c.dataPath, "catalog", "inventory.json")
	if _, err := os.Stat(jsonCatalogPath); os.IsNotExist(err) {
		return false, nil // No JSON catalog exists
	}
	
	// Check if SQLite database has any entries
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM catalog_entries").Scan(&count)
	if err != nil {
		return false, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to check SQLite catalog", err)
	}
	
	// Migration needed if JSON exists and SQLite is empty
	return count == 0, nil
}

// GetMigrationStatus returns information about the migration status
func (c *CatalogDB) GetMigrationStatus() (map[string]interface{}, error) {
	status := make(map[string]interface{})
	
	// Check JSON catalog
	jsonCatalogPath := filepath.Join(c.dataPath, "catalog", "inventory.json")
	jsonExists := false
	jsonEntries := 0
	
	if _, err := os.Stat(jsonCatalogPath); err == nil {
		jsonExists = true
		if catalog, err := c.loadJSONCatalog(jsonCatalogPath); err == nil {
			jsonEntries = len(catalog.Entries)
		}
	}
	
	// Check SQLite catalog
	var sqliteEntries int
	err := c.db.QueryRow("SELECT COUNT(*) FROM catalog_entries").Scan(&sqliteEntries)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "Failed to count SQLite entries", err)
	}
	
	// Check for migrated JSON catalog
	migratedPath := jsonCatalogPath + ".migrated"
	migratedExists := false
	if _, err := os.Stat(migratedPath); err == nil {
		migratedExists = true
	}
	
	status["json_catalog_exists"] = jsonExists
	status["json_entries"] = jsonEntries
	status["sqlite_entries"] = sqliteEntries
	status["migrated_catalog_exists"] = migratedExists
	status["migration_needed"] = jsonExists && sqliteEntries == 0
	
	return status, nil
}
