package artifact

import (
	"os"
	"path/filepath"
	"testing"

	"pudl/internal/database"
)

func setupTestDB(t *testing.T) (*database.CatalogDB, string) {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := database.NewCatalogDB(tmpDir)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	return db, tmpDir
}

func TestStoreBasic(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	dataPath := filepath.Join(tmpDir, "data")

	result, err := Store(db, StoreOptions{
		Definition: "test_def",
		Method:     "list",
		Output:     map[string]string{"hello": "world"},
		Tags:       map[string]string{"env": "staging"},
		DataPath:   dataPath,
	})
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if result.Proquint == "" {
		t.Error("expected non-empty proquint")
	}
	if result.Deduped {
		t.Error("first store should not be deduped")
	}

	// Verify file exists
	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("artifact file should exist: %v", err)
	}

	// Verify .meta sidecar exists
	metaPath := result.Path + ".meta"
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("meta file should exist: %v", err)
	}
}

func TestStoreDedup(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	dataPath := filepath.Join(tmpDir, "data")
	opts := StoreOptions{
		Definition: "test_def",
		Method:     "list",
		Output:     map[string]string{"same": "content"},
		DataPath:   dataPath,
	}

	first, err := Store(db, opts)
	if err != nil {
		t.Fatalf("first Store failed: %v", err)
	}
	if first.Deduped {
		t.Error("first store should not be deduped")
	}

	second, err := Store(db, opts)
	if err != nil {
		t.Fatalf("second Store failed: %v", err)
	}
	if !second.Deduped {
		t.Error("second store with same content should be deduped")
	}
	if first.ID != second.ID {
		t.Error("deduped entry should have same ID")
	}
}

func TestStoreTags(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	dataPath := filepath.Join(tmpDir, "data")

	result, err := Store(db, StoreOptions{
		Definition: "tagged_def",
		Method:     "create",
		Output:     "tagged output",
		Tags:       map[string]string{"env": "prod", "region": "us-east-1"},
		DataPath:   dataPath,
	})
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify we can retrieve it with tags
	entry, err := db.GetLatestArtifact("tagged_def", "create")
	if err != nil {
		t.Fatalf("GetLatestArtifact failed: %v", err)
	}

	if entry.ID != result.ID {
		t.Error("retrieved entry should match stored entry")
	}
	if entry.Tags == nil {
		t.Error("tags should be stored")
	}
}
