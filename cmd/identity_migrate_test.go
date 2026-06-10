package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/inference"
)

func strptr(s string) *string { return &s }

// TestAssignVersions covers the merge case: several entries that now share one
// resource_id must get a coherent monotonic version sequence ordered by import
// time, and the assignment must be idempotent.
func TestAssignVersions(t *testing.T) {
	t1 := time.Unix(1000, 0)
	t2 := time.Unix(2000, 0)
	t3 := time.Unix(3000, 0)

	// Three entries collapsed under resource_id "R" (out of time order in the
	// slice), plus a singleton under "S".
	entries := []database.CatalogEntry{
		{ID: "c", ResourceID: strptr("R"), ImportTimestamp: t3},
		{ID: "a", ResourceID: strptr("R"), ImportTimestamp: t1},
		{ID: "b", ResourceID: strptr("R"), ImportTimestamp: t2},
		{ID: "s", ResourceID: strptr("S"), ImportTimestamp: t2},
	}

	assignVersions(entries)

	want := map[string]int{"a": 1, "b": 2, "c": 3, "s": 1}
	got := map[string]int{}
	for _, e := range entries {
		if e.Version == nil {
			t.Fatalf("entry %s has nil version", e.ID)
		}
		got[e.ID] = *e.Version
	}
	for id, v := range want {
		if got[id] != v {
			t.Errorf("entry %s version = %d, want %d", id, got[id], v)
		}
	}

	// Idempotent: a second pass yields the same versions.
	assignVersions(entries)
	for _, e := range entries {
		if *e.Version != want[e.ID] {
			t.Errorf("after second pass, entry %s version = %d, want %d", e.ID, *e.Version, want[e.ID])
		}
	}
}

// TestMigrateEntryIdentity verifies resource_id is namespaced by the family root
// (a leaf schema resolves to its root) and that catchall entries use the content
// hash without reading the data file.
func TestMigrateEntryIdentity(t *testing.T) {
	dir := t.TempDir()

	// Minimal loadable CUE module: a base + a child in the same family, plus a
	// catchall.
	modFile := filepath.Join(dir, "cue.mod", "module.cue")
	if err := os.MkdirAll(filepath.Dir(modFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modFile, []byte("module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeFile := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("fam/base.cue", `package fam

#Base: {
	_pudl: {
		schema_type: "base"
		resource_type: "fam.base"
		identity_fields: ["id"]
	}
	id: string
	...
}
`)
	writeFile("fam/child.cue", `package fam

#Child: {
	_pudl: {
		schema_type: "base"
		resource_type: "fam.child"
		base_schema: "fam.#Base"
		identity_fields: ["id"]
	}
	id: string
	...
}
`)
	writeFile("unknown/catchall.cue", `package unknown

#CatchAll: {
	_pudl: {
		schema_type: "catchall"
		identity_fields: []
	}
	...
}
`)

	inferrer, err := inference.NewSchemaInferrer(dir)
	if err != nil {
		t.Fatalf("NewSchemaInferrer: %v", err)
	}
	graph := inferrer.GetInheritanceGraph()

	t.Run("leaf schema uses family root namespace", func(t *testing.T) {
		// Write the entry's data file with the identity value.
		dataPath := filepath.Join(dir, "data.json")
		if err := os.WriteFile(dataPath, []byte(`{"id":"x1"}`), 0644); err != nil {
			t.Fatal(err)
		}
		entry := &database.CatalogEntry{
			ID:          "entry1",
			Schema:      "fam.#Child",
			StoredPath:  dataPath,
			ContentHash: strptr("hash1"),
		}

		if err := migrateEntryIdentity(entry, inferrer, graph); err != nil {
			t.Fatalf("migrateEntryIdentity: %v", err)
		}

		// Expected: namespaced by the root (fam.#Base), not the leaf (fam.#Child).
		want := identity.ComputeResourceID("fam.#Base", map[string]interface{}{"id": "x1"}, "hash1")
		if entry.ResourceID == nil || *entry.ResourceID != want {
			t.Fatalf("resource_id = %v, want %s", entry.ResourceID, want)
		}
		// Sanity: differs from a leaf-namespaced id.
		leaf := identity.ComputeResourceID("fam.#Child", map[string]interface{}{"id": "x1"}, "hash1")
		if *entry.ResourceID == leaf {
			t.Fatal("resource_id should not equal the leaf-namespaced id")
		}
	})

	t.Run("catchall uses content hash without reading file", func(t *testing.T) {
		entry := &database.CatalogEntry{
			ID:          "entry2",
			Schema:      "unknown.#CatchAll",
			StoredPath:  filepath.Join(dir, "does-not-exist.json"),
			ContentHash: strptr("hash2"),
		}

		if err := migrateEntryIdentity(entry, inferrer, graph); err != nil {
			t.Fatalf("migrateEntryIdentity: %v", err)
		}

		want := identity.ComputeResourceID("unknown.#CatchAll", nil, "hash2")
		if entry.ResourceID == nil || *entry.ResourceID != want {
			t.Fatalf("resource_id = %v, want %s", entry.ResourceID, want)
		}
		if entry.IdentityJSON != nil {
			t.Fatalf("catchall identity_json should be nil, got %v", *entry.IdentityJSON)
		}
	})
}
