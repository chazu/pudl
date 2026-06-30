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

func TestDropLegacyMethodColumn(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Simulate a pre-cleanup database: re-add the `method` column, its index,
	// and a catalog_entry_edb view that references it. The view reference is the
	// reason cleanup must drop the view before the column (SQLite blocks
	// DROP COLUMN while a view references it).
	for _, stmt := range []string{
		"ALTER TABLE catalog_entries ADD COLUMN method TEXT",
		// idx_definition_method keeps its historical name (the index
		// dropLegacyMethodColumn drops); its columns are the post-rename `target`.
		"CREATE INDEX idx_definition_method ON catalog_entries(target, method)",
		"DROP VIEW IF EXISTS " + CatalogEntryView,
		"CREATE VIEW " + CatalogEntryView + " AS SELECT id, target, method FROM catalog_entries",
	} {
		if _, err := db.db.Exec(stmt); err != nil {
			t.Fatalf("legacy-state setup %q failed: %v", stmt, err)
		}
	}

	if exists, err := db.columnExists("catalog_entries", "method"); err != nil || !exists {
		t.Fatalf("precondition: method column should exist (exists=%v, err=%v)", exists, err)
	}

	if err := db.dropLegacyMethodColumn(); err != nil {
		t.Fatalf("dropLegacyMethodColumn failed: %v", err)
	}

	exists, err := db.columnExists("catalog_entries", "method")
	if err != nil {
		t.Fatalf("column check failed: %v", err)
	}
	if exists {
		t.Error("method column should be dropped after cleanup")
	}

	// Idempotent: a second run on the already-cleaned DB is a no-op.
	if err := db.dropLegacyMethodColumn(); err != nil {
		t.Fatalf("second dropLegacyMethodColumn run failed: %v", err)
	}

	// Leave the DB consistent: recreate the real view (no method column).
	if err := db.ensureCatalogEntryView(); err != nil {
		t.Fatalf("ensureCatalogEntryView failed: %v", err)
	}
}

func TestRenameLegacyDefinitionColumn(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Seed a row through the normal API (column is `target` post-migration), so
	// the value rides along when we rename the column to simulate a legacy DB.
	target := "models/foo"
	if err := db.AddEntry(CatalogEntry{
		ID: "id-foo", StoredPath: "/raw/foo", MetadataPath: "/meta/foo",
		ImportTimestamp: time.Now(), Format: "json", Origin: "test",
		Schema: "pudl/core.#Item", Confidence: 1.0, RecordCount: 1, SizeBytes: 1,
		Target: &target,
	}); err != nil {
		t.Fatalf("seed AddEntry failed: %v", err)
	}

	// Simulate a pre-rename database: drop the view + index that reference the
	// column, then rename `target` back to `definition` (the legacy name). The
	// seeded value moves with the column.
	for _, stmt := range []string{
		"DROP VIEW IF EXISTS " + CatalogEntryView,
		"DROP INDEX IF EXISTS idx_target",
		"ALTER TABLE catalog_entries RENAME COLUMN target TO definition",
	} {
		if _, err := db.db.Exec(stmt); err != nil {
			t.Fatalf("legacy-state setup %q failed: %v", stmt, err)
		}
	}
	if exists, _ := db.columnExists("catalog_entries", "definition"); !exists {
		t.Fatal("precondition: definition column should exist")
	}

	if err := db.renameLegacyDefinitionColumn(); err != nil {
		t.Fatalf("renameLegacyDefinitionColumn failed: %v", err)
	}

	// definition gone, target present, value preserved.
	if exists, _ := db.columnExists("catalog_entries", "definition"); exists {
		t.Error("definition column should be gone after rename")
	}
	if exists, _ := db.columnExists("catalog_entries", "target"); !exists {
		t.Error("target column should exist after rename")
	}
	var got string
	if err := db.db.QueryRow("SELECT target FROM catalog_entries WHERE id = 'id-foo'").Scan(&got); err != nil {
		t.Fatalf("read back target failed: %v", err)
	}
	if got != target {
		t.Errorf("target value not preserved: got %q, want %q", got, target)
	}

	// Idempotent: a second run on the already-renamed DB is a no-op.
	if err := db.renameLegacyDefinitionColumn(); err != nil {
		t.Fatalf("second renameLegacyDefinitionColumn run failed: %v", err)
	}

	// Leave the DB consistent.
	if err := db.ensureCatalogEntryView(); err != nil {
		t.Fatalf("ensureCatalogEntryView failed: %v", err)
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
