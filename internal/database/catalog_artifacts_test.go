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
