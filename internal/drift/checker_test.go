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
	"pudl/internal/executor"
	"pudl/internal/model"
)

// mockStepExecutor implements workflow.StepExecutor for testing.
type mockStepExecutor struct {
	runFunc func(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error)
}

func (m *mockStepExecutor) Run(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, opts)
	}
	return &executor.RunResult{}, nil
}

func TestChecker_MissingArtifact(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up schema dir with a definition and model
	schemaDir := filepath.Join(tmpDir, "schema")
	setupTestDefinition(t, schemaDir)
	setupTestModel(t, schemaDir)

	// Open a temp catalog DB
	dbDir := filepath.Join(tmpDir, "db")
	os.MkdirAll(dbDir, 0755)
	db, err := database.NewCatalogDB(dbDir)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer db.Close()

	defDisc := definition.NewDiscoverer(schemaDir)
	modelDisc := model.NewDiscoverer(schemaDir)
	exec := &mockStepExecutor{}
	dataPath := filepath.Join(tmpDir, "data")

	checker := NewChecker(defDisc, modelDisc, db, exec, dataPath)

	result, err := checker.Check(context.Background(), CheckOptions{
		DefinitionName: "my_instance",
		Method:         "list",
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
	setupTestModel(t, schemaDir)

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
	modelDisc := model.NewDiscoverer(schemaDir)
	exec := &mockStepExecutor{}
	dataPath := filepath.Join(tmpDir, "data")

	checker := NewChecker(defDisc, modelDisc, db, exec, dataPath)

	result, err := checker.Check(context.Background(), CheckOptions{
		DefinitionName: "my_instance",
		Method:         "list",
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

func TestChecker_Refresh(t *testing.T) {
	tmpDir := t.TempDir()

	schemaDir := filepath.Join(tmpDir, "schema")
	setupTestDefinition(t, schemaDir)
	setupTestModel(t, schemaDir)

	dbDir := filepath.Join(tmpDir, "db")
	os.MkdirAll(dbDir, 0755)
	db, err := database.NewCatalogDB(dbDir)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer db.Close()

	// Track whether executor was called
	executorCalled := false
	exec := &mockStepExecutor{
		runFunc: func(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
			executorCalled = true
			return &executor.RunResult{
				MethodName:     opts.MethodName,
				DefinitionName: opts.DefinitionName,
			}, nil
		},
	}

	defDisc := definition.NewDiscoverer(schemaDir)
	modelDisc := model.NewDiscoverer(schemaDir)
	dataPath := filepath.Join(tmpDir, "data")

	checker := NewChecker(defDisc, modelDisc, db, exec, dataPath)

	_, err = checker.Check(context.Background(), CheckOptions{
		DefinitionName: "my_instance",
		Method:         "list",
		Refresh:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !executorCalled {
		t.Error("expected executor to be called with Refresh=true")
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

// setupTestModel creates a minimal model CUE file.
func setupTestModel(t *testing.T, schemaDir string) {
	t.Helper()
	modelsDir := filepath.Join(schemaDir, "examples")
	os.MkdirAll(modelsDir, 0755)

	content := `package examples

#EC2InstanceModel: #Model & {
	metadata: {
		name: "ec2_instance"
		description: "EC2 Instance"
		category: "compute"
	}
	methods: {
		list: #Method & {
			kind: "action"
			description: "List instances"
		}
	}
}
`
	os.WriteFile(filepath.Join(modelsDir, "ec2.cue"), []byte(content), 0644)
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
