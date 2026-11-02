package database

import "testing"

// The legacy item_schemas table survives so catalogs written by the retired
// typed-envelope import keep their rows and open unchanged.
func TestItemSchemasLegacyTableRemainsQueryable(t *testing.T) {
	s := NewDatabaseTestSuite(t)
	db, err := NewCatalogDB(s.TempDir)
	if err != nil {
		t.Fatalf("NewCatalogDB: %v", err)
	}
	s.cleanupFns = append(s.cleanupFns, func() error { return db.Close() })

	if _, err := db.DB().Exec(`INSERT INTO item_schemas (item_id, schema_ref, status) VALUES ('item-a', 'mu/aws@v1', 'unresolved')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM item_schemas`).Scan(&count); err != nil {
		t.Fatalf("query legacy rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("legacy rows = %d, want 1", count)
	}
}
