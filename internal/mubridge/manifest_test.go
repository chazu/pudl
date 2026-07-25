package mubridge

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/database"
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

func TestIngestManifest_ModelTag(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	result, err := IngestManifest(db, strings.NewReader(sampleManifest), "mu-build", tmpDir, "mymodel")
	if err != nil {
		t.Fatalf("IngestManifest failed: %v", err)
	}
	actions, err := db.GetManifestActions(result.RunID)
	if err != nil {
		t.Fatalf("GetManifestActions failed: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}
	for _, a := range actions {
		if a.Tags == nil {
			t.Fatalf("expected tags on %v", a.Target)
		}
		var tags map[string]interface{}
		if err := json.Unmarshal([]byte(*a.Tags), &tags); err != nil {
			t.Fatalf("unmarshal tags: %v", err)
		}
		if tags["model"] != "mymodel" {
			t.Errorf("expected tags.model=mymodel, got %v", tags["model"])
		}
	}

	// Without --model, no model tag is written.
	db2, tmp2 := setupTestDB(t)
	defer db2.Close()
	r2, err := IngestManifest(db2, strings.NewReader(sampleManifest), "mu-build", tmp2, "")
	if err != nil {
		t.Fatalf("IngestManifest failed: %v", err)
	}
	a2, _ := db2.GetManifestActions(r2.RunID)
	for _, a := range a2 {
		var tags map[string]interface{}
		_ = json.Unmarshal([]byte(*a.Tags), &tags)
		if _, ok := tags["model"]; ok {
			t.Errorf("expected no model tag without --model, got %v", tags["model"])
		}
	}
}

func TestIngestManifest_Basic(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	reader := strings.NewReader(sampleManifest)
	result, err := IngestManifest(db, reader, "mu-build", tmpDir, "")
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
		Schema:     "pudl/mu.#Manifest",
		EntryTypes: []string{"manifest"},
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
		if a.Target != nil && *a.Target == "api_server" {
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
		if a.Target != nil && *a.Target == "monitoring" {
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
		if a.Target != nil && *a.Target == "config_file" {
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

func TestIngestManifestWithRunID_AttachesEnclosingRun(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	result, err := IngestManifestWithRunID(db, strings.NewReader(sampleManifest), "mu-build", tmpDir, "mymodel", "run_test")
	if err != nil {
		t.Fatalf("IngestManifestWithRunID failed: %v", err)
	}
	if result.RunID != "run_test" {
		t.Fatalf("expected run_test, got %q", result.RunID)
	}
	actions, err := db.GetManifestActions(result.RunID)
	if err != nil {
		t.Fatalf("GetManifestActions failed: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}
	for _, action := range actions {
		if action.RunID == nil || *action.RunID != "run_test" {
			t.Errorf("expected action run_id=run_test, got %v", action.RunID)
		}
	}
}

func TestIngestManifest_Dedup(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	// First ingestion
	reader1 := strings.NewReader(sampleManifest)
	result1, err := IngestManifest(db, reader1, "mu-build", tmpDir, "")
	if err != nil {
		t.Fatalf("first IngestManifest failed: %v", err)
	}
	if result1.Skipped {
		t.Error("first ingestion should not be skipped")
	}

	// Second ingestion of the same manifest
	reader2 := strings.NewReader(sampleManifest)
	result2, err := IngestManifest(db, reader2, "mu-build", tmpDir, "")
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

// A duplicate manifest is the same apply recorded twice, so re-ingesting must not
// rewrite statuses wholesale: a resource the drift re-check has since verified
// would be knocked from `clean` back to `converging`, undoing the verification
// with information older than it.
func TestIngestManifest_DedupDoesNotRegressVerifiedStatus(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	_, err := IngestManifest(db, strings.NewReader(sampleManifest), "mu-build", tmpDir, "")
	if err != nil {
		t.Fatalf("first IngestManifest failed: %v", err)
	}

	// The drift re-check verifies the applied resource.
	if err := db.UpdateStatus("api_server", "clean"); err != nil {
		t.Fatalf("promote to clean failed: %v", err)
	}

	result, err := IngestManifest(db, strings.NewReader(sampleManifest), "mu-build", tmpDir, "")
	if err != nil {
		t.Fatalf("re-ingest failed: %v", err)
	}
	if !result.Skipped {
		t.Fatal("re-ingest should be skipped as a duplicate")
	}
	if result.StatusesRepaired != 0 {
		t.Errorf("nothing needed repair, got StatusesRepaired=%d", result.StatusesRepaired)
	}

	statuses := targetStatusMap(t, db)
	if got := statuses["api_server"]; got != "clean" {
		t.Errorf("verified status must survive a duplicate ingest, got %q", got)
	}
	if got := statuses["config_file"]; got != "failed" {
		t.Errorf("failed action keeps its status, got %q", got)
	}
}

// The first ingest treats an UpdateStatus failure as a warning, so an action's
// apply can be recorded while its resource is left at the default `unknown`.
// Re-ingesting is the repair path for exactly that, and only that.
func TestIngestManifest_DedupRepairsUnwrittenStatus(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	if _, err := IngestManifest(db, strings.NewReader(sampleManifest), "mu-build", tmpDir, ""); err != nil {
		t.Fatalf("first IngestManifest failed: %v", err)
	}

	// Simulate the status write the first ingest never managed to land.
	if err := db.UpdateStatus("monitoring", "unknown"); err != nil {
		t.Fatalf("reset status failed: %v", err)
	}

	result, err := IngestManifest(db, strings.NewReader(sampleManifest), "mu-build", tmpDir, "")
	if err != nil {
		t.Fatalf("re-ingest failed: %v", err)
	}
	if result.StatusesRepaired != 1 {
		t.Errorf("expected 1 repaired status, got %d", result.StatusesRepaired)
	}
	if got := targetStatusMap(t, db)["monitoring"]; got != "converging" {
		t.Errorf("unwritten status should be repaired to converging, got %q", got)
	}
}

// A duplicate reports the run that owns the manifest, not the caller's — the
// entry is content-addressed and its run association is first-writer-wins.
func TestIngestManifest_DedupReportsOwningRun(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	first, err := IngestManifestWithRunID(db, strings.NewReader(sampleManifest), "mu-build", tmpDir, "", "run_first")
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	if first.RunID != "run_first" {
		t.Fatalf("expected run_first, got %q", first.RunID)
	}

	second, err := IngestManifestWithRunID(db, strings.NewReader(sampleManifest), "mu-build", tmpDir, "", "run_second")
	if err != nil {
		t.Fatalf("re-ingest failed: %v", err)
	}
	if !second.Skipped {
		t.Fatal("re-ingest should be skipped")
	}
	if second.RunID != "run_first" {
		t.Errorf("a duplicate names the run that owns the manifest, got %q", second.RunID)
	}
}

// A manifest ingest is one step. Failing part-way through must leave nothing —
// not the manifest, not the actions that already landed, not their statuses.
// A half-recorded apply is worse than none: the catalog shows work was done
// while the resources it touched still read `unknown`.
//
// The failure is forced by two byte-identical actions, which compute the same
// content-addressed entry ID and so collide on insert — the manifest entry and
// the first action are already written by then.
func TestIngestManifest_StepIsAtomic(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	collidingManifest := `{
  "timestamp": "2026-03-24T11:00:00Z",
  "summary": {"total": 2, "cached": 0, "executed": 2, "failed": 0},
  "actions": [
    {"id": "dup", "target": "//web", "cached": false, "exit_code": 0, "outputs": {}},
    {"id": "dup", "target": "//web", "cached": false, "exit_code": 0, "outputs": {}}
  ]
}`

	_, err := IngestManifest(db, strings.NewReader(collidingManifest), "mu-build", tmpDir, "")
	if err == nil {
		t.Fatal("expected the colliding action to fail the step")
	}

	entries, qErr := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	if qErr != nil {
		t.Fatalf("QueryEntries failed: %v", qErr)
	}
	if entries.FilteredCount != 0 {
		t.Errorf("a failed step must record no entries, got %d", entries.FilteredCount)
	}

	if status, ok := targetStatusMap(t, db)["web"]; ok {
		t.Errorf("a failed step must record no status, got %q", status)
	}
}

func targetStatusMap(t *testing.T, db *database.CatalogDB) map[string]string {
	t.Helper()
	statuses, err := db.GetTargetStatuses()
	if err != nil {
		t.Fatalf("GetTargetStatuses failed: %v", err)
	}
	got := make(map[string]string, len(statuses))
	for _, s := range statuses {
		got[s.Target] = s.Status
	}
	return got
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
	result, err := IngestManifest(db, reader, "mu-build", tmpDir, "")
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

func TestIngestManifest_NormalizeTarget(t *testing.T) {
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
		got := normalizeTarget(tt.target)
		if got != tt.expected {
			t.Errorf("normalizeTarget(%q) = %q, want %q", tt.target, got, tt.expected)
		}
	}
}

func TestGetLatestManifestAction(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer db.Close()

	reader := strings.NewReader(sampleManifest)
	_, err := IngestManifest(db, reader, "mu-build", tmpDir, "")
	if err != nil {
		t.Fatalf("IngestManifest failed: %v", err)
	}

	// Get latest manifest action for api_server
	latest, err := db.GetLatestManifestAction("api_server")
	if err != nil {
		t.Fatalf("GetLatestManifestAction failed: %v", err)
	}
	if latest.Target == nil || *latest.Target != "api_server" {
		t.Errorf("expected target=api_server, got %v", latest.Target)
	}

	// Non-existent target should return error
	_, err = db.GetLatestManifestAction("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent target")
	}
}
