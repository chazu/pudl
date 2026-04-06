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

	// mu observe --json output: array of ObserveResult with current.records
	input := `[
		{
			"target": "//home/odroid",
			"current": {
				"records": [
					{"_schema": "linux.host", "hostname": "renge", "kernel": "5.10.0", "arch": "aarch64", "os": {"id": "debian", "version": "10", "name": "Debian"}, "uptime_seconds": 12114},
					{"_schema": "linux.package", "host": "renge", "name": "acl", "version": "2.2.53-4", "status": "ii "},
					{"_schema": "linux.package", "host": "renge", "name": "adduser", "version": "3.118", "status": "ii "},
					{"_schema": "linux.service", "host": "renge", "unit": "ssh.service", "active": "active", "sub": "running"}
				]
			}
		}
	]`

	count, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("IngestObserveResults failed: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 ingested records, got %d", count)
	}

	// All records should be stored as observe entries for target "home/odroid"
	entry, err := db.GetLatestObserve("home/odroid")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected observe entry for home/odroid")
	}
	if entry.EntryType == nil || *entry.EntryType != "observe" {
		t.Errorf("expected entry_type 'observe', got %v", entry.EntryType)
	}
	if entry.Origin != "mu-observe" {
		t.Errorf("expected origin 'mu-observe', got %s", entry.Origin)
	}
}

func TestIngestObserveResults_SchemaRouting(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `[
		{
			"target": "//home/renge",
			"current": {
				"records": [
					{"_schema": "linux.host", "hostname": "renge", "kernel": "5.10.0", "arch": "aarch64", "os": {"id": "debian", "version": "10", "name": "Debian"}, "uptime_seconds": 100},
					{"_schema": "linux.service", "host": "renge", "unit": "cron.service", "active": "active", "sub": "running"}
				]
			}
		}
	]`

	count, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("IngestObserveResults failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 records, got %d", count)
	}

	// Query all observe entries and check schemas were routed correctly
	entries, err := db.QueryEntries(
		database.FilterOptions{EntryType: "observe"},
		database.QueryOptions{Limit: 100},
	)
	if err != nil {
		t.Fatalf("QueryEntries failed: %v", err)
	}
	if entries.FilteredCount != 2 {
		t.Fatalf("expected 2 entries, got %d", entries.FilteredCount)
	}

	schemas := map[string]bool{}
	for _, e := range entries.Entries {
		schemas[e.Schema] = true
	}
	if !schemas["pudl/linux.#Host"] {
		t.Error("expected pudl/linux.#Host schema in results")
	}
	if !schemas["pudl/linux.#Service"] {
		t.Error("expected pudl/linux.#Service schema in results")
	}
}

func TestIngestObserveResults_Dedup(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `[{"target":"//app","current":{"records":[{"_schema":"linux.host","hostname":"box","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":1}]}}]`

	count1, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	if count1 != 1 {
		t.Errorf("expected 1 on first ingest, got %d", count1)
	}

	// Same data again — should deduplicate
	count2, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if count2 != 0 {
		t.Errorf("expected 0 on duplicate ingest, got %d", count2)
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
		t.Errorf("expected 0 for empty input, got %d", count)
	}
}

func TestIngestObserveResults_TargetError(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	// Targets with errors should be skipped, not ingested
	input := `[
		{"target":"//broken","error":"plugin crashed"},
		{"target":"//ok","current":{"records":[{"_schema":"linux.host","hostname":"good","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":1}]}}
	]`

	count, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 (skipping errored target), got %d", count)
	}
}

func TestIngestObserveResults_NoRecordsKey(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	// current without records key — treat whole current as single record
	input := `[{"target":"//simple","current":{"status":"healthy","uptime":42}}]`

	count, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 record, got %d", count)
	}

	entry, err := db.GetLatestObserve("simple")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected observe entry for simple")
	}
	// No _schema field, should fall back to generic observe result
	if entry.Schema != "pudl/mu.#ObserveResult" {
		t.Errorf("expected fallback schema, got %s", entry.Schema)
	}
}

func TestIngestObserveResults_MultipleTargets(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `[
		{"target":"//host/a","current":{"records":[{"_schema":"linux.host","hostname":"a","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":1}]}},
		{"target":"//host/b","current":{"records":[{"_schema":"linux.host","hostname":"b","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":2}]}}
	]`

	count, err := IngestObserveResults(db, strings.NewReader(input), "mu-observe", dataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 records (one per target), got %d", count)
	}

	// Both targets should have entries
	e1, _ := db.GetLatestObserve("host/a")
	e2, _ := db.GetLatestObserve("host/b")
	if e1 == nil {
		t.Error("expected observe entry for host/a")
	}
	if e2 == nil {
		t.Error("expected observe entry for host/b")
	}
}

func TestIngestObserveResults_CustomOrigin(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `[{"target":"//app","current":{"records":[{"_schema":"linux.host","hostname":"x","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":1}]}}]`

	count, err := IngestObserveResults(db, strings.NewReader(input), "custom-origin", dataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	entry, _ := db.GetLatestObserve("app")
	if entry == nil {
		t.Fatal("expected entry")
	}
	if entry.Origin != "custom-origin" {
		t.Errorf("expected origin 'custom-origin', got %s", entry.Origin)
	}
}

func TestResourceTypeToSchema(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"linux.host", "pudl/linux.#Host"},
		{"linux.package", "pudl/linux.#Package"},
		{"linux.network_interface", "pudl/linux.#NetworkInterface"},
		{"linux.service", "pudl/linux.#Service"},
		{"linux.filesystem", "pudl/linux.#Filesystem"},
		{"linux.user", "pudl/linux.#User"},
		{"aws.ec2_instance", "pudl/aws.#Ec2Instance"},
		{"unknown", "pudl/mu.#ObserveResult"},  // no dot separator
	}

	for _, tt := range tests {
		got := resourceTypeToSchema(tt.input)
		if got != tt.expected {
			t.Errorf("resourceTypeToSchema(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
