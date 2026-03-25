package database

import (
	"testing"
	"time"
)

func addTestObserve(t *testing.T, db *CatalogDB, id, def, contentHash string) {
	t.Helper()
	entryType := "observe"
	schema := "pudl/mu.#ObserveResult"
	entry := CatalogEntry{
		ID:             id,
		StoredPath:     "/tmp/test/" + id + ".json",
		MetadataPath:   "/tmp/test/" + id + ".json.meta",
		ImportTimestamp: time.Now(),
		Format:         "json",
		Origin:         "mu-observe",
		Schema:         schema,
		Confidence:     1.0,
		RecordCount:    1,
		SizeBytes:      100,
		EntryType:      &entryType,
		Definition:     &def,
		ContentHash:    &contentHash,
	}
	if err := db.AddEntry(entry); err != nil {
		t.Fatalf("failed to add test observe entry: %v", err)
	}
}

func TestGetLatestObserve(t *testing.T) {
	db := setupArtifactTestDB(t)
	defer db.Close()

	addTestObserve(t, db, "obs111obs111obs111obs111obs111obs111obs111obs111obs111obs111obs1", "my_app", "hash1")

	time.Sleep(10 * time.Millisecond)
	addTestObserve(t, db, "obs222obs222obs222obs222obs222obs222obs222obs222obs222obs222obs2", "my_app", "hash2")

	latest, err := db.GetLatestObserve("my_app")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if latest == nil {
		t.Fatal("expected non-nil entry")
	}
	if latest.ID != "obs222obs222obs222obs222obs222obs222obs222obs222obs222obs222obs2" {
		t.Errorf("expected latest observe entry, got %s", latest.ID)
	}
	if latest.EntryType == nil || *latest.EntryType != "observe" {
		t.Errorf("expected entry_type 'observe', got %v", latest.EntryType)
	}
}

func TestGetLatestObserve_NoResults(t *testing.T) {
	db := setupArtifactTestDB(t)
	defer db.Close()

	entry, err := db.GetLatestObserve("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Error("expected nil entry for nonexistent definition")
	}
}

func TestGetLatestObserve_DoesNotReturnArtifacts(t *testing.T) {
	db := setupArtifactTestDB(t)
	defer db.Close()

	// Add an artifact, not an observe
	addTestArtifact(t, db, "art111art111art111art111art111art111art111art111art111art111art1", "my_app", "list")

	entry, err := db.GetLatestObserve("my_app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Error("expected nil entry — artifacts should not be returned by GetLatestObserve")
	}
}

func TestGetLatestObserveByContentHash(t *testing.T) {
	db := setupArtifactTestDB(t)
	defer db.Close()

	addTestObserve(t, db, "obs333obs333obs333obs333obs333obs333obs333obs333obs333obs333obs3", "my_app", "abcdef1234")

	// Should find existing
	entry, err := db.GetLatestObserveByContentHash("my_app", "abcdef1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected to find observe entry by content hash")
	}

	// Should not find non-matching hash
	entry, err = db.GetLatestObserveByContentHash("my_app", "different_hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Error("expected nil for non-matching content hash")
	}
}
