package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pudl/internal/database"
	"pudl/internal/mubridge"
	"pudl/internal/muschemas"
)

// TestEndToEnd_AutoRegisterThenDeclared simulates the full
// sidecar-driven flow:
//
//  1. First import: a data file + sidecar carrying inline CUE
//     definitions. The schema cache is empty; classifySidecar should
//     auto-register the definitions and report status=auto_registered.
//
//  2. Second import of a different data file with the same module+version:
//     the cache is now populated, so classifySidecar should report
//     status=declared (no re-registration needed).
//
//  3. Reclassify: a row that was originally tagged as unresolved (e.g.
//     because the first import had no inline definitions) is upgraded
//     to declared once the schema is in the cache.
func TestEndToEnd_AutoRegisterThenDeclared(t *testing.T) {
	dir := t.TempDir()
	cache, err := muschemas.New(filepath.Join(dir, "schemas"))
	if err != nil {
		t.Fatalf("muschemas.New: %v", err)
	}

	// Synthesize a data file and a sidecar with inline definitions.
	dataA := filepath.Join(dir, "instance-a.json")
	if err := os.WriteFile(dataA, []byte(`{"instance_id":"i-aaa"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecarA := mubridge.SchemaSidecar{
		Module:  "mu/aws",
		Version: "v1",
		Definitions: []mubridge.SidecarDefinitionFile{
			{Path: "ec2.cue", Content: "package aws\n#EC2Instance: {instance_id: string}\n"},
		},
	}
	mustWriteSidecar(t, dataA, sidecarA)

	// First import — cache is empty, sidecar carries definitions.
	statusA, err := classifySidecar(cache, &sidecarA)
	if err != nil {
		t.Fatalf("first import classifySidecar: %v", err)
	}
	if statusA != database.ItemSchemaStatusAutoRegistered {
		t.Errorf("first import status = %q, want auto_registered", statusA)
	}
	if !cache.Has("mu/aws", "v1") {
		t.Fatal("expected cache.Has after auto-register")
	}

	// Second import of a different data file, same (module, version).
	// Cache hit → status=declared; sidecar definitions re-supplied
	// (idempotent insert path).
	dataB := filepath.Join(dir, "instance-b.json")
	os.WriteFile(dataB, []byte(`{"instance_id":"i-bbb"}`), 0o644)
	sidecarB := sidecarA // same module/version/definitions
	statusB, err := classifySidecar(cache, &sidecarB)
	if err != nil {
		t.Fatalf("second import classifySidecar: %v", err)
	}
	if statusB != database.ItemSchemaStatusDeclared {
		t.Errorf("second import status = %q, want declared", statusB)
	}

	// Reclassify path: simulate a third import that recorded an
	// unresolved row (e.g. no definitions in its sidecar) before the
	// cache learned this schema. Now that the cache knows it,
	// tryResolveSchemaRef should report true.
	resolved, err := tryResolveSchemaRef(cache, "mu/aws@v1#EC2Instance")
	if err != nil {
		t.Fatalf("tryResolveSchemaRef: %v", err)
	}
	if !resolved {
		t.Error("expected tryResolveSchemaRef to succeed after auto-register")
	}
}

// TestEndToEnd_ReclassifyUpgradesUnresolved exercises the database
// side: an item starts with an unresolved row, the schema is later
// added to the cache, and the row is upgraded to declared.
func TestEndToEnd_ReclassifyUpgradesUnresolved(t *testing.T) {
	tdir := t.TempDir()
	db, err := database.NewCatalogDB(tdir)
	if err != nil {
		t.Fatalf("NewCatalogDB: %v", err)
	}
	defer db.Close()

	cache, err := muschemas.New(filepath.Join(tdir, "schemas"))
	if err != nil {
		t.Fatalf("muschemas.New: %v", err)
	}

	// Initial row: schema was unknown at import time.
	row := database.ItemSchema{
		ItemID:    "item-xyz",
		SchemaRef: "mu/aws@v1",
		Status:    database.ItemSchemaStatusUnresolved,
	}
	if err := db.AddItemSchema(row); err != nil {
		t.Fatalf("AddItemSchema: %v", err)
	}

	// Schema arrives later (e.g. via a second import that carried
	// inline definitions, or a manual cache populate).
	if err := cache.Insert("mu/aws", "v1", []muschemas.File{
		{RelPath: "ec2.cue", Content: []byte("package aws\n#EC2Instance: {}\n")},
	}); err != nil {
		t.Fatalf("cache.Insert: %v", err)
	}

	// Reclassify the unresolved row.
	rows, err := db.ListUnresolvedItemSchemas("")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListUnresolved: got %d, want 1", len(rows))
	}
	resolved, err := tryResolveSchemaRef(cache, rows[0].SchemaRef)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved {
		t.Fatal("ref should resolve now")
	}
	if err := db.AddItemSchema(database.ItemSchema{
		ItemID:    rows[0].ItemID,
		SchemaRef: rows[0].SchemaRef,
		Status:    database.ItemSchemaStatusDeclared,
	}); err != nil {
		t.Fatal(err)
	}

	// Verify the upgrade landed.
	got, err := db.ListItemSchemas("item-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1 (upsert should not duplicate)", len(got))
	}
	if got[0].Status != database.ItemSchemaStatusDeclared {
		t.Errorf("status = %q, want declared", got[0].Status)
	}

	// And: nothing left to reclassify.
	leftover, err := db.ListUnresolvedItemSchemas("")
	if err != nil {
		t.Fatal(err)
	}
	if len(leftover) != 0 {
		t.Errorf("leftover unresolved = %d, want 0", len(leftover))
	}
}

// TestEndToEnd_SidecarRoundTrip writes a sidecar to disk and reads it
// back, exercising the wire format (mu-side writer and pudl-side
// reader agree).
func TestEndToEnd_SidecarRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "out.json")
	os.WriteFile(dataPath, []byte("{}"), 0o644)

	want := mubridge.SchemaSidecar{
		Module: "mu/aws", Version: "v1", Definition: "#EC2Instance",
		Definitions: []mubridge.SidecarDefinitionFile{
			{Path: "ec2.cue", Content: "package aws\n#EC2Instance: {}\n"},
			{Path: "vpc/vpc.cue", Content: "package vpc\n#VPC: {}\n"},
		},
	}
	mustWriteSidecar(t, dataPath, want)

	got, err := mubridge.ReadSidecar(dataPath)
	if err != nil {
		t.Fatalf("ReadSidecar: %v", err)
	}
	if got.Module != want.Module || got.Version != want.Version || got.Definition != want.Definition {
		t.Errorf("ref fields mismatch: got %+v want %+v", got, want)
	}
	if len(got.Definitions) != len(want.Definitions) {
		t.Fatalf("Definitions len = %d, want %d", len(got.Definitions), len(want.Definitions))
	}
	for i := range got.Definitions {
		if got.Definitions[i] != want.Definitions[i] {
			t.Errorf("Definitions[%d] mismatch: got %+v want %+v", i, got.Definitions[i], want.Definitions[i])
		}
	}
}

func mustWriteSidecar(t *testing.T, dataPath string, side mubridge.SchemaSidecar) {
	t.Helper()
	b, err := json.MarshalIndent(side, "", "  ")
	if err != nil {
		t.Fatalf("marshal sidecar: %v", err)
	}
	if err := os.WriteFile(mubridge.SidecarPath(dataPath), b, 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
}
