package mubridge

import (
	"encoding/json"
	"strings"
	"testing"

	"pudl/internal/database"
)

const sampleManifest = `{
  "timestamp": "2026-03-24T10:15:00Z",
  "summary": {"total": 3, "cached": 1, "executed": 2, "failed": 1},
  "actions": [
    {"id": "abc123", "target": "//api_server", "cached": false, "exit_code": 0, "outputs": {}},
    {"id": "def456", "target": "//monitoring", "cached": true, "exit_code": 0, "outputs": {}},
    {"id": "ghi789", "target": "//config_file", "cached": false, "exit_code": 1, "outputs": {}}
  ]
}`

func setupTestDB(t *testing.T) (*database.CatalogDB, string) {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := database.NewCatalogDB(tmpDir)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	return db, tmpDir
}

func TestIngestManifest_Basic(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	reader := strings.NewReader(sampleManifest)
	result, err := IngestManifest(db, reader, "mu-build", tmpDir)
	if err != nil {
		t.Fatalf("IngestManifest failed: %v", err)
	}

	if result.Skipped {
		t.Error("expected manifest to not be skipped")
	}
	if result.RunID == "" {
		t.Error("expected non-empty run_id")
	}
	if result.Total != 3 {
		t.Errorf("expected total=3, got %d", result.Total)
	}
	if result.Cached != 1 {
		t.Errorf("expected cached=1, got %d", result.Cached)
	}
	if result.Failed != 1 {
		t.Errorf("expected failed=1, got %d", result.Failed)
	}

	// Verify manifest entry exists
	manifestEntries, err := db.QueryEntries(database.FilterOptions{
		Schema:    "pudl/mu.#Manifest",
		EntryType: "manifest",
	}, database.QueryOptions{})
	if err != nil {
		t.Fatalf("QueryEntries failed: %v", err)
	}
	if manifestEntries.FilteredCount != 1 {
		t.Errorf("expected 1 manifest entry, got %d", manifestEntries.FilteredCount)
	}

	// Verify manifest-action entries exist
	actions, err := db.GetManifestActions(result.RunID)
	if err != nil {
		t.Fatalf("GetManifestActions failed: %v", err)
	}
	if len(actions) != 3 {
		t.Errorf("expected 3 manifest-action entries, got %d", len(actions))
	}

	// All should share the same run_id
	for _, a := range actions {
		if a.RunID == nil || *a.RunID != result.RunID {
			t.Errorf("expected run_id=%s, got %v", result.RunID, a.RunID)
		}
		if a.EntryType == nil || *a.EntryType != "manifest-action" {
			t.Errorf("expected entry_type=manifest-action, got %v", a.EntryType)
		}
	}

	// Verify tags contain correct exit_code and cached values
	// Find the api_server action (exit_code=0, cached=false)
	for _, a := range actions {
		if a.Definition != nil && *a.Definition == "api_server" {
			if a.Tags == nil {
				t.Fatal("expected tags on api_server action")
			}
			var tags map[string]interface{}
			if err := json.Unmarshal([]byte(*a.Tags), &tags); err != nil {
				t.Fatalf("failed to unmarshal tags: %v", err)
			}
			if tags["exit_code"] != float64(0) {
				t.Errorf("expected exit_code=0 for api_server, got %v", tags["exit_code"])
			}
			if tags["cached"] != false {
				t.Errorf("expected cached=false for api_server, got %v", tags["cached"])
			}
		}
		if a.Definition != nil && *a.Definition == "monitoring" {
			if a.Tags == nil {
				t.Fatal("expected tags on monitoring action")
			}
			var tags map[string]interface{}
			if err := json.Unmarshal([]byte(*a.Tags), &tags); err != nil {
				t.Fatalf("failed to unmarshal tags: %v", err)
			}
			if tags["cached"] != true {
				t.Errorf("expected cached=true for monitoring, got %v", tags["cached"])
			}
		}
		if a.Definition != nil && *a.Definition == "config_file" {
			if a.Tags == nil {
				t.Fatal("expected tags on config_file action")
			}
			var tags map[string]interface{}
			if err := json.Unmarshal([]byte(*a.Tags), &tags); err != nil {
				t.Fatalf("failed to unmarshal tags: %v", err)
			}
			if tags["exit_code"] != float64(1) {
				t.Errorf("expected exit_code=1 for config_file, got %v", tags["exit_code"])
			}
		}
	}
}

func TestIngestManifest_Dedup(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	// First ingestion
	reader1 := strings.NewReader(sampleManifest)
	result1, err := IngestManifest(db, reader1, "mu-build", tmpDir)
	if err != nil {
		t.Fatalf("first IngestManifest failed: %v", err)
	}
	if result1.Skipped {
		t.Error("first ingestion should not be skipped")
	}

	// Second ingestion of the same manifest
	reader2 := strings.NewReader(sampleManifest)
	result2, err := IngestManifest(db, reader2, "mu-build", tmpDir)
	if err != nil {
		t.Fatalf("second IngestManifest failed: %v", err)
	}
	if !result2.Skipped {
		t.Error("second ingestion should be skipped (duplicate)")
	}

	// Verify only one set of entries exists
	allEntries, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	if err != nil {
		t.Fatalf("QueryEntries failed: %v", err)
	}
	// 1 manifest + 3 actions = 4 total
	if allEntries.FilteredCount != 4 {
		t.Errorf("expected 4 total entries after dedup, got %d", allEntries.FilteredCount)
	}
}

func TestIngestManifest_EmptyActions(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	emptyManifest := `{
		"timestamp": "2026-03-24T11:00:00Z",
		"summary": {"total": 0, "cached": 0, "executed": 0, "failed": 0},
		"actions": []
	}`

	reader := strings.NewReader(emptyManifest)
	result, err := IngestManifest(db, reader, "mu-build", tmpDir)
	if err != nil {
		t.Fatalf("IngestManifest failed: %v", err)
	}

	if result.Skipped {
		t.Error("expected manifest to not be skipped")
	}

	// Verify only the manifest entry exists
	allEntries, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	if err != nil {
		t.Fatalf("QueryEntries failed: %v", err)
	}
	if allEntries.FilteredCount != 1 {
		t.Errorf("expected 1 entry (manifest only), got %d", allEntries.FilteredCount)
	}

	// Verify no actions
	actions, err := db.GetManifestActions(result.RunID)
	if err != nil {
		t.Fatalf("GetManifestActions failed: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}
}

func TestIngestManifest_TargetToDefinition(t *testing.T) {
	tests := []struct {
		target   string
		expected string
	}{
		{"//my_app", "my_app"},
		{"//api_server", "api_server"},
		{"my_app", "my_app"},
		{"//", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := targetToDefinition(tt.target)
		if got != tt.expected {
			t.Errorf("targetToDefinition(%q) = %q, want %q", tt.target, got, tt.expected)
		}
	}
}

func TestGetLatestManifestAction(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	reader := strings.NewReader(sampleManifest)
	_, err := IngestManifest(db, reader, "mu-build", tmpDir)
	if err != nil {
		t.Fatalf("IngestManifest failed: %v", err)
	}

	// Get latest manifest action for api_server
	latest, err := db.GetLatestManifestAction("api_server")
	if err != nil {
		t.Fatalf("GetLatestManifestAction failed: %v", err)
	}
	if latest.Definition == nil || *latest.Definition != "api_server" {
		t.Errorf("expected definition=api_server, got %v", latest.Definition)
	}

	// Non-existent definition should return error
	_, err = db.GetLatestManifestAction("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent definition")
	}
}
