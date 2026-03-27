package mubridge

import (
	"path/filepath"
	"strings"
	"testing"

	"pudl/internal/database"
)

func setupIngestTestDB(t *testing.T) (*database.CatalogDB, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbDir := filepath.Join(tmpDir, "db")
	db, err := database.NewCatalogDB(dbDir)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	dataDir := filepath.Join(tmpDir, "data")
	return db, dataDir
}

func TestIngestObserveResults_Basic(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `{"target":"//my_app","state":"converged"}
{"target":"//my_db","state":"drifted","diff":"replicas: 3 -> 2"}
`
	reader := strings.NewReader(input)

	count, err := IngestObserveResults(db, reader, "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("IngestObserveResults failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 ingested results, got %d", count)
	}

	// Verify first entry
	entry1, err := db.GetLatestObserve("my_app")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry1 == nil {
		t.Fatal("expected observe entry for my_app")
	}
	if entry1.EntryType == nil || *entry1.EntryType != "observe" {
		t.Errorf("expected entry_type 'observe', got %v", entry1.EntryType)
	}
	if entry1.Definition == nil || *entry1.Definition != "my_app" {
		t.Errorf("expected definition 'my_app', got %v", entry1.Definition)
	}
	if entry1.Schema != "pudl/mu.#ObserveResult" {
		t.Errorf("expected schema 'pudl/mu.#ObserveResult', got %s", entry1.Schema)
	}
	if entry1.Origin != "mu-observe" {
		t.Errorf("expected origin 'mu-observe', got %s", entry1.Origin)
	}
	if entry1.Format != "json" {
		t.Errorf("expected format 'json', got %s", entry1.Format)
	}

	// Verify second entry
	entry2, err := db.GetLatestObserve("my_db")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry2 == nil {
		t.Fatal("expected observe entry for my_db")
	}
	if entry2.Definition == nil || *entry2.Definition != "my_db" {
		t.Errorf("expected definition 'my_db', got %v", entry2.Definition)
	}
}

func TestIngestObserveResults_Dedup(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `{"target":"//my_app","state":"converged"}`

	// Ingest first time
	count1, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	if count1 != 1 {
		t.Errorf("expected 1 result on first ingest, got %d", count1)
	}

	// Ingest same data again — should be deduplicated
	count2, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if count2 != 0 {
		t.Errorf("expected 0 results on duplicate ingest, got %d", count2)
	}
}

func TestIngestObserveResults_EmptyInput(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	count, err := IngestObserveResults(db, strings.NewReader(""), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 results for empty input, got %d", count)
	}
}

func TestIngestObserveResults_InvalidJSON(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	// Mix of valid and invalid lines
	input := `not valid json
{"target":"//my_app","state":"converged"}
also not valid
{"target":"//my_db","state":"drifted","diff":"engine changed"}
`
	count, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 valid results (skipping invalid), got %d", count)
	}

	// Verify valid entries were stored
	entry1, err := db.GetLatestObserve("my_app")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry1 == nil {
		t.Error("expected observe entry for my_app")
	}

	entry2, err := db.GetLatestObserve("my_db")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry2 == nil {
		t.Error("expected observe entry for my_db")
	}
}

func TestIngestObserveResults_TargetWithoutPrefix(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	// Target without "//" prefix should also work
	input := `{"target":"my_app","state":"converged"}`
	count, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 result, got %d", count)
	}

	entry, err := db.GetLatestObserve("my_app")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected observe entry for my_app")
	}
	if entry.Definition == nil || *entry.Definition != "my_app" {
		t.Errorf("expected definition 'my_app', got %v", entry.Definition)
	}
}

func TestIngestObserveResults_CustomOrigin(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `{"target":"//my_app","state":"drifted","diff":"something changed"}`
	count, err := IngestObserveResults(db, strings.NewReader(input), "custom-origin", dataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 result, got %d", count)
	}

	entry, err := db.GetLatestObserve("my_app")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry.Origin != "custom-origin" {
		t.Errorf("expected origin 'custom-origin', got %s", entry.Origin)
	}
}
