package repair

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/inference"
)

// policyLeafSchemas is a type, a policy on it and a policy extending that one.
const policyLeafSchemas = `package fam

#Base: {
	_pudl: {schema_type: "base", resource_type: "fam.base", identity_fields: ["id"]}
	id: string
	...
}

#Hardened: {
	_pudl: {schema_type: "policy", base_schema: "fam.#Base"}
	encrypted: true
	...
}

#Strict: {
	_pudl: {schema_type: "policy", base_schema: "fam.#Hardened"}
	versioned: true
	...
}
`

// TestPolicyLeavesRepair covers a catalog fixture with entries classified to
// policy leaves before policies left classification: diagnosis and dry run
// find them without writing, the fix moves each to its type, and identity,
// version history and the other entries are untouched.
func TestPolicyLeavesRepair(t *testing.T) {
	dir := t.TempDir()
	for rel, content := range map[string]string{
		"cue.mod/module.cue": "module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n",
		"fam/fam.cue":        policyLeafSchemas,
	} {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	inferrer, err := inference.NewSchemaInferrer(dir)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.NewCatalogDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// One resource observed three times: as the type, then (under the old
	// rules) as each policy. Its resource_id is namespaced by the family root.
	values := map[string]interface{}{"id": "x1"}
	rid := identity.ComputeResourceID(inferrer.GetInheritanceGraph().IdentityRoot("fam.#Strict"), values, "unused")
	identityJSON, _ := identity.CanonicalIdentityJSON(values)
	schemas := map[string]string{"1111111100000000": "fam.#Base", "2222222200000000": "fam.#Hardened", "3333333300000000": "fam.#Strict"}
	for i, id := range []string{"1111111100000000", "2222222200000000", "3333333300000000"} {
		version := i + 1
		e := database.CatalogEntry{ID: id, Schema: schemas[id], Format: "json", Confidence: 0.9,
			StoredPath: filepath.Join(dir, id+".json"), ContentHash: strptr("hash-" + id),
			ResourceID: strptr(rid), IdentityJSON: strptr(identityJSON), Version: &version,
			ImportTimestamp: time.Unix(int64(1000*version), 0)}
		if err := db.AddEntry(e); err != nil {
			t.Fatal(err)
		}
	}
	env := &Env{DB: db, Inferrer: inferrer}
	ctx := context.Background()

	diag := Run(ctx, policyLeaves, env, false, false)
	if diag.Status != StatusNeeded || diag.Pending != 2 || len(diag.Changes) != 2 {
		t.Fatalf("diagnosis = %+v, want two policy-leaf entries", diag)
	}
	moves := map[string]string{}
	for _, c := range diag.Changes {
		moves[c.Subject] = c.From + " -> " + c.To
	}
	for id, want := range map[string]string{"2222222200000000": "fam.#Hardened -> fam.#Base", "3333333300000000": "fam.#Strict -> fam.#Base"} {
		if got := moves[idgen.HashToProquint(id)]; got != want {
			t.Errorf("move of %s = %q, want %q (all: %v)", id, got, want, moves)
		}
	}

	preview := Run(ctx, policyLeaves, env, true, true)
	if preview.Status != StatusNeeded || preview.Changed != 0 {
		t.Fatalf("dry run = %+v, want pending and nothing written", preview)
	}
	if got, _ := db.GetEntry("3333333300000000"); got.Schema != "fam.#Strict" {
		t.Fatal("dry run wrote")
	}

	applied := Run(ctx, policyLeaves, env, true, false)
	if applied.Status != StatusApplied || applied.Changed != 2 {
		t.Fatalf("apply = %+v, want two entries moved", applied)
	}
	for i, id := range []string{"1111111100000000", "2222222200000000", "3333333300000000"} {
		got, err := db.GetEntry(id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Schema != "fam.#Base" {
			t.Errorf("%s schema = %s, want fam.#Base", id, got.Schema)
		}
		if got.ResourceID == nil || *got.ResourceID != rid || got.IdentityJSON == nil || *got.IdentityJSON != identityJSON {
			t.Errorf("%s identity changed: %v %v", id, got.ResourceID, got.IdentityJSON)
		}
		if got.Version == nil || *got.Version != i+1 {
			t.Errorf("%s version = %v, want %d", id, got.Version, i+1)
		}
	}
	// The kept resource_id is the one an import under the type now derives.
	if want := identity.ComputeResourceID(inferrer.GetInheritanceGraph().IdentityRoot("fam.#Base"), values, "unused"); rid != want {
		t.Fatalf("resource_id %s differs from the type's %s", rid, want)
	}

	if again := Run(ctx, policyLeaves, env, false, false); again.Status != StatusOK || again.Pending != 0 {
		t.Fatalf("diagnosis after fix = %+v, want ok", again)
	}
}
