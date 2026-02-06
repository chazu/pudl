package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*CatalogDB, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "pudl-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	db, err := NewCatalogDB(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create catalog DB: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

func TestEnsureIdentityColumns_Idempotent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Run migration a second time — should not error
	if err := db.ensureIdentityColumns(); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}

	// Verify columns exist
	for _, col := range []string{"resource_id", "content_hash", "identity_json", "version"} {
		exists, err := db.columnExists("catalog_entries", col)
		if err != nil {
			t.Fatalf("column check failed for %s: %v", col, err)
		}
		if !exists {
			t.Errorf("column %s should exist after migration", col)
		}
	}
}

func TestBackfillDefaults(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pudl-test-backfill-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create database and add an entry without identity fields
	db, err := NewCatalogDB(tmpDir)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}

	entry := CatalogEntry{
		ID:              "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		StoredPath:      filepath.Join(tmpDir, "test.json"),
		MetadataPath:    filepath.Join(tmpDir, "test.meta"),
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test",
		Schema:          "pudl/core.#Item",
		Confidence:      0.5,
		RecordCount:     1,
		SizeBytes:       100,
		// ContentHash and Version intentionally nil
	}
	if err := db.AddEntry(entry); err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}
	db.Close()

	// Re-open database — backfill should run and fill in defaults
	db2, err := NewCatalogDB(tmpDir)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer db2.Close()

	retrieved, err := db2.GetEntry(entry.ID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	// content_hash should be backfilled to the entry's ID
	if retrieved.ContentHash == nil {
		t.Fatal("content_hash should not be nil after backfill")
	}
	if *retrieved.ContentHash != entry.ID {
		t.Errorf("content_hash should be backfilled to entry ID, got %s", *retrieved.ContentHash)
	}

	// version should be backfilled to 1
	if retrieved.Version == nil {
		t.Fatal("version should not be nil after backfill")
	}
	if *retrieved.Version != 1 {
		t.Errorf("version should be backfilled to 1, got %d", *retrieved.Version)
	}
}
