package pithdriver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chazu/pith"

	"pudl/internal/database"
	"pudl/internal/schema"
)

func setupTestDB(t *testing.T) (*database.CatalogDB, string) {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := database.NewCatalogDB(tmpDir)
	if err != nil {
		t.Fatalf("NewCatalogDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, tmpDir
}

func seedFleet(t *testing.T, db *database.CatalogDB) {
	t.Helper()
	states := []string{"running", "running", "running", "stopped", "stopped", "terminated"}
	for i, state := range states {
		status := state
		entry := database.CatalogEntry{
			ID:             fmt.Sprintf("inst-%d", i),
			StoredPath:     fmt.Sprintf("/data/ec2/%d.json", i),
			MetadataPath:   fmt.Sprintf("/data/ec2/%d.meta.json", i),
			ImportTimestamp: time.Now(),
			Format:         "json",
			Origin:         "aws",
			Schema:         "aws.#EC2Instance",
			Confidence:     1.0,
			RecordCount:    1,
			SizeBytes:      256,
			Status:         &status,
		}
		if err := db.AddEntry(entry); err != nil {
			t.Fatalf("AddEntry %d: %v", i, err)
		}
	}
}

// Example 1: Fleet Summary
// Query all EC2 instances, get total count and group by status.
func TestExampleFleetSummary(t *testing.T) {
	db, _ := setupTestDB(t)
	seedFleet(t, db)

	vm := pith.New(context.Background())
	Register(vm, db, nil)

	// Program: query EC2 instances, dup for count, group by status
	program := []any{
		map[string]any{"schema": "aws.#EC2Instance"},
		"catalog/query",
		"dup", "len",
		// stack: [entries] count
		// swap entries back to top for group-by
		"swap",
		[]any{"'status", "get"}, "group-by",
		// stack: count grouped_map
	}

	err := vm.Run(program)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	stack := vm.Stack()
	if len(stack) != 2 {
		t.Fatalf("stack depth = %d, want 2", len(stack))
	}

	count := stack[0]
	if count != 6 {
		t.Errorf("total count = %v, want 6", count)
	}

	groups, ok := stack[1].(map[string]any)
	if !ok {
		t.Fatalf("groups is %T, want map", stack[1])
	}

	running := groups["running"].([]any)
	stopped := groups["stopped"].([]any)
	terminated := groups["terminated"].([]any)

	if len(running) != 3 {
		t.Errorf("running = %d, want 3", len(running))
	}
	if len(stopped) != 2 {
		t.Errorf("stopped = %d, want 2", len(stopped))
	}
	if len(terminated) != 1 {
		t.Errorf("terminated = %d, want 1", len(terminated))
	}
}

// Example 2: Filter by status
// Query catalog, filter to only running instances, extract IDs.
func TestExampleFilterByStatus(t *testing.T) {
	db, _ := setupTestDB(t)
	seedFleet(t, db)

	vm := pith.New(context.Background())
	Register(vm, db, nil)

	program := []any{
		map[string]any{"schema": "aws.#EC2Instance"},
		"catalog/query",
		[]any{"'status", "get", "'running", "eq"}, "filter",
		[]any{"'id", "get"}, "map",
	}

	err := vm.Run(program)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, _ := vm.Result()
	ids := result.([]any)
	if len(ids) != 3 {
		t.Errorf("running IDs = %v, want 3 items", ids)
	}
}

// Example 3: Count by origin
// Query all entries, count matching a filter.
func TestExampleCountByOrigin(t *testing.T) {
	db, _ := setupTestDB(t)
	seedFleet(t, db)

	vm := pith.New(context.Background())
	Register(vm, db, nil)

	program := []any{
		map[string]any{"origin": "aws"}, "catalog/count",
	}

	err := vm.Run(program)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, _ := vm.Result()
	if result != 6 {
		t.Errorf("count = %v, want 6", result)
	}
}

// Example 4: Schema Discovery
// List schemas, extract package names.
func TestExampleSchemaDiscovery(t *testing.T) {
	tmpDir := t.TempDir()
	schemaDir := filepath.Join(tmpDir, "schemas")

	// Create mock schema files
	for _, pkg := range []string{"aws", "gcp"} {
		dir := filepath.Join(schemaDir, pkg)
		os.MkdirAll(dir, 0755)
		for _, name := range []string{"instance.cue", "network.cue"} {
			content := fmt.Sprintf("package %s\n\n#Instance: {\n\tid: string\n}\n", pkg)
			os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
		}
	}

	mgr := schema.NewManager(schemaDir)
	vm := pith.New(context.Background())
	Register(vm, nil, mgr)

	// Program: list schemas, get package names
	program := []any{
		"schema/list", "keys",
	}

	err := vm.Run(program)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, _ := vm.Result()
	packages := result.([]any)
	if len(packages) != 2 {
		t.Errorf("packages = %v, want 2 items", packages)
	}
}

// Example 5: Field refs with catalog query
// Use SetContext to provide input, resolve field refs in program.
func TestExampleFieldRefQuery(t *testing.T) {
	db, _ := setupTestDB(t)
	seedFleet(t, db)

	vm := pith.New(context.Background())
	Register(vm, db, nil)
	vm.SetContext("input", map[string]any{
		"schema": "aws.#EC2Instance",
		"origin": "aws",
	})

	// Program: build filter map from input refs, query
	program := []any{
		// set ( obj key value -- obj' )
		map[string]any{}, "'schema", "input.schema", "set",
		"'origin", "input.origin", "set",
		"catalog/count",
	}

	err := vm.Run(program)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, _ := vm.Result()
	if result != 6 {
		t.Errorf("count = %v, want 6", result)
	}
}
