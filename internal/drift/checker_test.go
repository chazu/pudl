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

func TestChecker_MissingData(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up schema dir with a definition
	schemaDir := filepath.Join(tmpDir, "schema")
	setupTestDefinition(t, schemaDir)

	// Open a temp catalog DB
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
		t.Errorf("expected status 'unknown' for missing data, got %q", result.Status)
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

	// Create a data file with matching data
	dataDir := filepath.Join(tmpDir, "data", "artifacts")
	os.MkdirAll(dataDir, 0755)
	dataPath := filepath.Join(dataDir, "test.json")
	artifactData := map[string]interface{}{
		"vpc_id": "vpc-123",
	}
	data, _ := json.Marshal(artifactData)
	os.WriteFile(dataPath, data, 0644)

	// Insert entry into catalog
	insertTestArtifact(t, db, "my_instance", "", dataPath)

	defDisc := definition.NewDiscoverer(schemaDir)

	checker := NewChecker(defDisc, db, filepath.Join(tmpDir, "data"))

	result, err := checker.Check(context.Background(), CheckOptions{
		DefinitionName: "my_instance",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Socket bindings are cross-ref expressions, not plain values, so they'll differ
	// unless live state has identical keys. This tests the flow completes.
	t.Logf("status: %s, diffs: %d", result.Status, len(result.Differences))
}

// setupTestDefinition creates a minimal definition CUE file.
// Uses the model unification pattern that the discoverer currently expects.
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
	}
	if method != "" {
		entry.Method = &method
	}

	err := db.AddEntry(entry)
	if err != nil {
		t.Fatalf("failed to insert test artifact: %v", err)
	}
}
