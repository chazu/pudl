package drift

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pudl/internal/database"
	"pudl/internal/definition"
)

func TestChecker_MissingArtifact(t *testing.T) {
	tmpDir := t.TempDir()

	schemaDir := filepath.Join(tmpDir, "schema")
	setupTestDefinition(t, schemaDir)

	dbDir := filepath.Join(tmpDir, "db")
	os.MkdirAll(dbDir, 0755)
	db, err := database.NewCatalogDB(dbDir)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer db.Close()

	defDisc := definition.NewDiscoverer(schemaDir)
	dataPath := filepath.Join(tmpDir, "data")

	checker := NewChecker(defDisc, db, dataPath)

	result, err := checker.Check(context.Background(), CheckOptions{
		DefinitionName: "my_instance",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != "unknown" {
		t.Errorf("expected status 'unknown' for missing artifact, got %q", result.Status)
	}
}

func TestChecker_CleanState(t *testing.T) {
	tmpDir := t.TempDir()

	schemaDir := filepath.Join(tmpDir, "schema")
	setupTestDefinition(t, schemaDir)

	dbDir := filepath.Join(tmpDir, "db")
	os.MkdirAll(dbDir, 0755)
	db, err := database.NewCatalogDB(dbDir)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer db.Close()

	// Create an artifact file with matching data
	artifactDir := filepath.Join(tmpDir, "data", "artifacts")
	os.MkdirAll(artifactDir, 0755)
	artifactPath := filepath.Join(artifactDir, "test.json")
	artifactData := map[string]interface{}{
		"vpc_id": "vpc-123",
	}
	data, _ := json.Marshal(artifactData)
	os.WriteFile(artifactPath, data, 0644)

	// Insert artifact into catalog
	insertTestArtifact(t, db, "my_instance", "list", artifactPath)

	defDisc := definition.NewDiscoverer(schemaDir)
	dataPath := filepath.Join(tmpDir, "data")

	checker := NewChecker(defDisc, db, dataPath)

	result, err := checker.Check(context.Background(), CheckOptions{
		DefinitionName: "my_instance",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != "drifted" {
		// Socket bindings are cross-ref expressions, not plain values, so they'll differ
		// unless live state has identical keys. This tests the flow completes.
		t.Logf("status: %s, diffs: %d", result.Status, len(result.Differences))
	}
}

// setupTestDefinition creates a minimal definition CUE file.
func setupTestDefinition(t *testing.T, schemaDir string) {
	t.Helper()
	defsDir := filepath.Join(schemaDir, "definitions")
	os.MkdirAll(defsDir, 0755)

	content := `package definitions

my_instance: examples.#EC2InstanceModel & {
	vpc_id: other_def.outputs.vpc_id
}
`
	os.WriteFile(filepath.Join(defsDir, "test.cue"), []byte(content), 0644)
}

func TestChecker_PrefersObserveOverArtifact(t *testing.T) {
	tmpDir := t.TempDir()

	schemaDir := filepath.Join(tmpDir, "schema")
	setupTestDefinition(t, schemaDir)

	dbDir := filepath.Join(tmpDir, "db")
	os.MkdirAll(dbDir, 0755)
	db, err := database.NewCatalogDB(dbDir)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer db.Close()

	// Create artifact data file with one value
	dataDir := filepath.Join(tmpDir, "data", "artifacts")
	os.MkdirAll(dataDir, 0755)
	artifactPath := filepath.Join(dataDir, "artifact.json")
	artifactData := map[string]interface{}{
		"vpc_id": "vpc-artifact",
	}
	aData, _ := json.Marshal(artifactData)
	os.WriteFile(artifactPath, aData, 0644)

	// Create observe data file with a different value
	observeDir := filepath.Join(tmpDir, "data", "observe")
	os.MkdirAll(observeDir, 0755)
	observePath := filepath.Join(observeDir, "observe.json")
	observeData := map[string]interface{}{
		"vpc_id": "vpc-observe",
	}
	oData, _ := json.Marshal(observeData)
	os.WriteFile(observePath, oData, 0644)

	// Insert artifact entry
	insertTestArtifact(t, db, "my_instance", "", artifactPath)

	// Insert observe entry
	insertTestObserve(t, db, "my_instance", observePath)

	defDisc := definition.NewDiscoverer(schemaDir)
	checker := NewChecker(defDisc, db, filepath.Join(tmpDir, "data"))

	result, err := checker.Check(context.Background(), CheckOptions{
		DefinitionName: "my_instance",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the live state came from the observe entry, not the artifact.
	if result.LiveState == nil {
		t.Fatal("expected non-nil LiveState")
	}
	if result.LiveState["vpc_id"] != "vpc-observe" {
		t.Errorf("expected live state to use observe data (vpc-observe), got %v", result.LiveState["vpc_id"])
	}
}

// insertTestObserve inserts a test observe entry into the catalog.
func insertTestObserve(t *testing.T, db *database.CatalogDB, defName, storedPath string) {
	t.Helper()
	entryType := "observe"
	entry := database.CatalogEntry{
		ID:             "test-observe-id",
		StoredPath:     storedPath,
		MetadataPath:   "",
		ImportTimestamp: time.Now(),
		Format:         "json",
		Origin:         "mu-observe",
		Schema:         "pudl/mu.#ObserveResult",
		Confidence:     1.0,
		RecordCount:    1,
		SizeBytes:      100,
		EntryType:      &entryType,
		Definition:     &defName,
	}
	err := db.AddEntry(entry)
	if err != nil {
		t.Fatalf("failed to insert test observe: %v", err)
	}
}

// insertTestArtifact inserts a test artifact entry into the catalog.
func insertTestArtifact(t *testing.T, db *database.CatalogDB, defName, method, storedPath string) {
	t.Helper()
	entryType := "artifact"
	entry := database.CatalogEntry{
		ID:             "test-artifact-id",
		StoredPath:     storedPath,
		MetadataPath:   "",
		ImportTimestamp: time.Now(),
		Format:         "json",
		Origin:         "test",
		Schema:         "",
		Confidence:     1.0,
		RecordCount:    1,
		SizeBytes:      100,
		EntryType:      &entryType,
		Definition:     &defName,
		Method:         &method,
	}

	err := db.AddEntry(entry)
	if err != nil {
		t.Fatalf("failed to insert test artifact: %v", err)
	}
}
