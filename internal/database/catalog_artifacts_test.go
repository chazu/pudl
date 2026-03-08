package database

import (
	"testing"
	"time"
)

func setupArtifactTestDB(t *testing.T) *CatalogDB {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := NewCatalogDB(tmpDir)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	return db
}

func addTestArtifact(t *testing.T, db *CatalogDB, id, def, method string) {
	t.Helper()
	entryType := "artifact"
	entry := CatalogEntry{
		ID:             id,
		StoredPath:     "/tmp/test/" + id + ".json",
		MetadataPath:   "/tmp/test/" + id + ".json.meta",
		ImportTimestamp: time.Now(),
		Format:         "json",
		Origin:         "method:" + def + "." + method,
		Schema:         "pudl/artifact",
		Confidence:     1.0,
		RecordCount:    1,
		SizeBytes:      100,
		EntryType:      &entryType,
		Definition:     &def,
		Method:         &method,
	}
	if err := db.AddEntry(entry); err != nil {
		t.Fatalf("failed to add test artifact: %v", err)
	}
}

func TestGetLatestArtifact(t *testing.T) {
	db := setupArtifactTestDB(t)
	defer db.Close()

	addTestArtifact(t, db, "aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa1", "mydef", "list")

	// Small delay to ensure different timestamps
	time.Sleep(10 * time.Millisecond)
	addTestArtifact(t, db, "bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb2", "mydef", "list")

	latest, err := db.GetLatestArtifact("mydef", "list")
	if err != nil {
		t.Fatalf("GetLatestArtifact failed: %v", err)
	}
	if latest.ID != "bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb2" {
		t.Errorf("expected latest artifact, got %s", latest.ID)
	}
}

func TestGetLatestArtifactNotFound(t *testing.T) {
	db := setupArtifactTestDB(t)
	defer db.Close()

	_, err := db.GetLatestArtifact("nonexistent", "method")
	if err == nil {
		t.Error("expected error for nonexistent artifact")
	}
}

func TestSearchArtifacts(t *testing.T) {
	db := setupArtifactTestDB(t)
	defer db.Close()

	addTestArtifact(t, db, "aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa1", "def1", "list")
	addTestArtifact(t, db, "bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb2", "def1", "create")
	addTestArtifact(t, db, "ccc333ccc333ccc333ccc333ccc333ccc333ccc333ccc333ccc333ccc333ccc3", "def2", "list")

	// Search all artifacts
	all, err := db.SearchArtifacts(ArtifactFilters{})
	if err != nil {
		t.Fatalf("SearchArtifacts failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 artifacts, got %d", len(all))
	}

	// Search by definition
	def1, err := db.SearchArtifacts(ArtifactFilters{Definition: "def1"})
	if err != nil {
		t.Fatalf("SearchArtifacts failed: %v", err)
	}
	if len(def1) != 2 {
		t.Errorf("expected 2 artifacts for def1, got %d", len(def1))
	}

	// Search by definition + method
	def1List, err := db.SearchArtifacts(ArtifactFilters{Definition: "def1", Method: "list"})
	if err != nil {
		t.Fatalf("SearchArtifacts failed: %v", err)
	}
	if len(def1List) != 1 {
		t.Errorf("expected 1 artifact for def1.list, got %d", len(def1List))
	}
}

func TestSearchArtifactsDoesNotReturnImports(t *testing.T) {
	db := setupArtifactTestDB(t)
	defer db.Close()

	// Add an import entry (default entry_type)
	importType := "import"
	entry := CatalogEntry{
		ID:             "ddd444ddd444ddd444ddd444ddd444ddd444ddd444ddd444ddd444ddd444ddd4",
		StoredPath:     "/tmp/test/import.json",
		MetadataPath:   "/tmp/test/import.json.meta",
		ImportTimestamp: time.Now(),
		Format:         "json",
		Origin:         "test",
		Schema:         "test.#Schema",
		Confidence:     0.9,
		RecordCount:    10,
		SizeBytes:      500,
		EntryType:      &importType,
	}
	if err := db.AddEntry(entry); err != nil {
		t.Fatalf("failed to add import entry: %v", err)
	}

	// Search artifacts should not return the import
	results, err := db.SearchArtifacts(ArtifactFilters{})
	if err != nil {
		t.Fatalf("SearchArtifacts failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(results))
	}
}
