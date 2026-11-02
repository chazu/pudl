package database

import (
	"os"
	"path/filepath"
	"testing"
)

// A new catalog, and the -wal and -shm files SQLite creates beside it, are
// owner-only under an owner-only directory.
func TestNewCatalogIsOwnerOnly(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := NewCatalogDB(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DB().Exec("CREATE TABLE perms_probe (x)"); err != nil {
		t.Fatal(err)
	}
	sqliteDir := filepath.Join(tmpDir, "data", "sqlite")
	assertMode(t, sqliteDir, 0o700)
	for _, name := range []string{"catalog.db", "catalog.db-wal", "catalog.db-shm"} {
		assertMode(t, filepath.Join(sqliteDir, name), 0o600)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", path, got, want)
	}
}
