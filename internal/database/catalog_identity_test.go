package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFindByContentHash(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	contentHash := "deadbeef1234567890abcdef1234567890abcdef1234567890abcdef12345678"
	entry := CatalogEntry{
		ID:              "aaaa0000bbbb1111cccc2222dddd3333eeee4444ffff5555aaaa0000bbbb1111",
		StoredPath:      filepath.Join("test", "data.json"),
		MetadataPath:    filepath.Join("test", "data.meta"),
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test",
		Schema:          "pudl/core.#Item",
		Confidence:      0.8,
		RecordCount:     1,
		SizeBytes:       100,
		ContentHash:     &contentHash,
	}
	if err := db.AddEntry(entry); err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}

	// Find by content hash
	found, err := db.FindByContentHash(contentHash)
	if err != nil {
		t.Fatalf("FindByContentHash failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find entry by content hash")
	}
	if found.ID != entry.ID {
		t.Errorf("expected ID %s, got %s", entry.ID, found.ID)
	}

	// Not found case
	notFound, err := db.FindByContentHash("nonexistent")
	if err != nil {
		t.Fatalf("FindByContentHash failed for nonexistent: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for nonexistent content hash")
	}
}

func TestFindByResourceID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	resourceID := "res123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd"
	v1 := 1
	v2 := 2

	hash1 := "hash1111111111111111111111111111111111111111111111111111111111111"
	hash2 := "hash2222222222222222222222222222222222222222222222222222222222222"

	entry1 := CatalogEntry{
		ID:              "e1111111111111111111111111111111111111111111111111111111111111111",
		StoredPath:      "test/v1.json",
		MetadataPath:    "test/v1.meta",
		ImportTimestamp: time.Now().Add(-time.Hour),
		Format:          "json",
		Origin:          "test",
		Schema:          "aws/ec2.#Instance",
		Confidence:      0.9,
		RecordCount:     1,
		SizeBytes:       100,
		ResourceID:      &resourceID,
		ContentHash:     &hash1,
		Version:         &v1,
	}
	entry2 := CatalogEntry{
		ID:              "e2222222222222222222222222222222222222222222222222222222222222222",
		StoredPath:      "test/v2.json",
		MetadataPath:    "test/v2.meta",
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test",
		Schema:          "aws/ec2.#Instance",
		Confidence:      0.9,
		RecordCount:     1,
		SizeBytes:       150,
		ResourceID:      &resourceID,
		ContentHash:     &hash2,
		Version:         &v2,
	}

	if err := db.AddEntry(entry1); err != nil {
		t.Fatalf("failed to add entry1: %v", err)
	}
	if err := db.AddEntry(entry2); err != nil {
		t.Fatalf("failed to add entry2: %v", err)
	}

	// Find by resource ID — should return both, newest first
	entries, err := db.FindByResourceID(resourceID)
	if err != nil {
		t.Fatalf("FindByResourceID failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if *entries[0].Version != 2 {
		t.Errorf("expected first entry to be version 2, got %d", *entries[0].Version)
	}
	if *entries[1].Version != 1 {
		t.Errorf("expected second entry to be version 1, got %d", *entries[1].Version)
	}
}

func TestGetLatestVersion(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	resourceID := "latestver123456789abcdef0123456789abcdef0123456789abcdef01234567"

	// No entries — version should be 0
	version, err := db.GetLatestVersion(resourceID)
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}
	if version != 0 {
		t.Errorf("expected version 0 for nonexistent resource, got %d", version)
	}

	// Add version 1
	v1 := 1
	hash := "somehash12345678901234567890123456789012345678901234567890123456"
	entry := CatalogEntry{
		ID:              "latest111111111111111111111111111111111111111111111111111111111111",
		StoredPath:      "test/data.json",
		MetadataPath:    "test/data.meta",
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test",
		Schema:          "pudl/core.#Item",
		Confidence:      0.5,
		RecordCount:     1,
		SizeBytes:       50,
		ResourceID:      &resourceID,
		ContentHash:     &hash,
		Version:         &v1,
	}
	if err := db.AddEntry(entry); err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}

	version, err = db.GetLatestVersion(resourceID)
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}

	// Add version 3 (skipping 2 is fine — it's just the highest)
	v3 := 3
	hash2 := "anotherhash45678901234567890123456789012345678901234567890123456"
	entry2 := CatalogEntry{
		ID:              "latest333333333333333333333333333333333333333333333333333333333333",
		StoredPath:      "test/data3.json",
		MetadataPath:    "test/data3.meta",
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test",
		Schema:          "pudl/core.#Item",
		Confidence:      0.5,
		RecordCount:     1,
		SizeBytes:       50,
		ResourceID:      &resourceID,
		ContentHash:     &hash2,
		Version:         &v3,
	}
	if err := db.AddEntry(entry2); err != nil {
		t.Fatalf("failed to add entry2: %v", err)
	}

	version, err = db.GetLatestVersion(resourceID)
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}
	if version != 3 {
		t.Errorf("expected version 3, got %d", version)
	}
}

func TestInsertEntryWithIdentityColumns(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	resourceID := "resid000111222333444555666777888999aaabbbcccdddeeefffggghhhiiijjj"
	contentHash := "hash000111222333444555666777888999aaabbbcccdddeeefffggghhhiiijjj"
	identityJSON := `{"id":"abc-123","region":"us-east-1"}`
	version := 1

	entry := CatalogEntry{
		ID:              "full0000111122223333444455556666777788889999aaaabbbbccccddddeeeeff",
		StoredPath:      "test/full.json",
		MetadataPath:    "test/full.meta",
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test-origin",
		Schema:          "aws/ec2.#Instance",
		Confidence:      0.95,
		RecordCount:     1,
		SizeBytes:       200,
		ResourceID:      &resourceID,
		ContentHash:     &contentHash,
		IdentityJSON:    &identityJSON,
		Version:         &version,
	}

	if err := db.AddEntry(entry); err != nil {
		t.Fatalf("failed to add entry with identity columns: %v", err)
	}

	retrieved, err := db.GetEntry(entry.ID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	if retrieved.ResourceID == nil || *retrieved.ResourceID != resourceID {
		t.Errorf("resource_id mismatch: expected %s, got %v", resourceID, retrieved.ResourceID)
	}
	if retrieved.ContentHash == nil || *retrieved.ContentHash != contentHash {
		t.Errorf("content_hash mismatch: expected %s, got %v", contentHash, retrieved.ContentHash)
	}
	if retrieved.IdentityJSON == nil || *retrieved.IdentityJSON != identityJSON {
		t.Errorf("identity_json mismatch: expected %s, got %v", identityJSON, retrieved.IdentityJSON)
	}
	if retrieved.Version == nil || *retrieved.Version != version {
		t.Errorf("version mismatch: expected %d, got %v", version, retrieved.Version)
	}
}
