package repair

import (
	"context"
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

// identityFixture writes a schema family (base + child) and a catchall, and
// returns the schema directory and its inferrer.
func identityFixture(t *testing.T) (string, *inference.SchemaInferrer) {
	t.Helper()
	dir := t.TempDir()
	writeFile := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("cue.mod/module.cue", "module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n")
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
	return dir, inferrer
}

// TestIdentityRepair covers the former `migrate identity` backfill and
// `--recompute`: a missing resource_id is backfilled under the family root, a
// leaf-namespaced one is recomputed and merged into one version history,
// catchall entries are identified by content hash without reading their
// payload, and a second diagnosis finds nothing.
func TestIdentityRepair(t *testing.T) {
	dir, inferrer := identityFixture(t)
	db, err := database.NewCatalogDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, []byte(`{"id":"x1"}`), 0644); err != nil {
		t.Fatal(err)
	}
	values := map[string]interface{}{"id": "x1"}
	// Same identity, so root namespacing merges base and child into one resource.
	rootRID := identity.ComputeResourceID("fam.#Base", values, "hash-base")
	leafRID := identity.ComputeResourceID("fam.#Child", values, "hash-child")
	one := 1
	for _, e := range []database.CatalogEntry{
		{ID: "base", Schema: "fam.#Base", StoredPath: dataPath, Format: "json", ContentHash: strptr("hash-base"), ResourceID: &rootRID, Version: &one, ImportTimestamp: time.Unix(1000, 0)},
		{ID: "child", Schema: "fam.#Child", StoredPath: dataPath, Format: "json", ContentHash: strptr("hash-child"), ResourceID: &leafRID, Version: &one, ImportTimestamp: time.Unix(2000, 0)},
		{ID: "missing", Schema: "fam.#Child", StoredPath: dataPath, Format: "json", ContentHash: strptr("hash-missing"), ImportTimestamp: time.Unix(3000, 0)},
		{ID: "catchall", Schema: "unknown.#CatchAll", StoredPath: filepath.Join(dir, "absent.json"), Format: "json", ContentHash: strptr("hash-catchall"), ImportTimestamp: time.Unix(4000, 0)},
	} {
		if err := db.AddEntry(e); err != nil {
			t.Fatal(err)
		}
	}
	env := &Env{DB: db, Inferrer: inferrer}

	preview := Run(context.Background(), identities, env, true, true)
	if preview.Status != StatusNeeded || preview.Pending != 3 || preview.Changed != 0 {
		t.Fatalf("dry run = %+v, want 3 pending and nothing written", preview)
	}
	if got, _ := db.GetEntry("child"); *got.ResourceID != leafRID {
		t.Fatal("dry run wrote")
	}

	applied := Run(context.Background(), identities, env, true, false)
	if applied.Status != StatusApplied || applied.Changed != 3 {
		t.Fatalf("apply = %+v, want 3 applied", applied)
	}
	child, _ := db.GetEntry("child")
	if *child.ResourceID != rootRID {
		t.Fatalf("child resource_id = %s, want the root-namespaced %s", *child.ResourceID, rootRID)
	}
	if child.Version == nil || *child.Version != 2 {
		t.Fatalf("child version = %v, want 2 after merging into the base resource", child.Version)
	}
	missing, _ := db.GetEntry("missing")
	if missing.ResourceID == nil || *missing.ResourceID != rootRID {
		t.Fatalf("missing resource_id = %v, want %s", missing.ResourceID, rootRID)
	}
	catchall, _ := db.GetEntry("catchall")
	if want := identity.ComputeResourceID("unknown.#CatchAll", nil, "hash-catchall"); catchall.ResourceID == nil || *catchall.ResourceID != want || catchall.IdentityJSON != nil {
		t.Fatalf("catchall identity = %v %v, want %s and no identity_json", catchall.ResourceID, catchall.IdentityJSON, want)
	}

	again := Run(context.Background(), identities, env, false, false)
	if again.Status != StatusOK || again.Pending != 0 {
		t.Fatalf("diagnosis after repair = %+v, want ok", again)
	}
}
