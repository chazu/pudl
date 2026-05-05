package cmd

import (
	"testing"

	"pudl/internal/database"
	"pudl/internal/mubridge"
	"pudl/internal/muschemas"
)

func newTestCache(t *testing.T) *muschemas.Cache {
	t.Helper()
	c, err := muschemas.New(t.TempDir())
	if err != nil {
		t.Fatalf("muschemas.New: %v", err)
	}
	return c
}

func TestClassifyEnvelope_Unresolved(t *testing.T) {
	cache := newTestCache(t)
	env := &mubridge.Envelope{
		Schema: mubridge.EnvelopeSchema{Module: "mu/aws", Version: "v1"},
	}
	status, err := classifyEnvelopeSchema(cache, env)
	if err != nil {
		t.Fatal(err)
	}
	if status != database.ItemSchemaStatusUnresolved {
		t.Errorf("status = %q, want unresolved", status)
	}
}

func TestClassifyEnvelope_AutoRegistered(t *testing.T) {
	cache := newTestCache(t)
	env := &mubridge.Envelope{
		Schema: mubridge.EnvelopeSchema{Module: "mu/aws", Version: "v1"},
		Definitions: []mubridge.EnvelopeDefFile{
			{Path: "ec2.cue", Content: "package aws\n#EC2Instance: {}\n"},
		},
	}
	status, err := classifyEnvelopeSchema(cache, env)
	if err != nil {
		t.Fatal(err)
	}
	if status != database.ItemSchemaStatusAutoRegistered {
		t.Errorf("status = %q, want auto_registered", status)
	}
	if !cache.Has("mu/aws", "v1") {
		t.Error("expected cache.Has after auto-register")
	}
}

func TestClassifyEnvelope_Declared(t *testing.T) {
	cache := newTestCache(t)
	if err := cache.Insert("mu/aws", "v1", []muschemas.File{
		{RelPath: "ec2.cue", Content: []byte("package aws\n#EC2Instance: {}\n")},
	}); err != nil {
		t.Fatal(err)
	}
	env := &mubridge.Envelope{Schema: mubridge.EnvelopeSchema{Module: "mu/aws", Version: "v1"}}
	status, err := classifyEnvelopeSchema(cache, env)
	if err != nil {
		t.Fatal(err)
	}
	if status != database.ItemSchemaStatusDeclared {
		t.Errorf("status = %q, want declared", status)
	}
}

func TestClassifyEnvelope_DefinitionsConflict(t *testing.T) {
	cache := newTestCache(t)
	if err := cache.Insert("mu/aws", "v1", []muschemas.File{
		{RelPath: "ec2.cue", Content: []byte("package aws\n#EC2Instance: {}\n")},
	}); err != nil {
		t.Fatal(err)
	}
	env := &mubridge.Envelope{
		Schema: mubridge.EnvelopeSchema{Module: "mu/aws", Version: "v1"},
		Definitions: []mubridge.EnvelopeDefFile{
			{Path: "ec2.cue", Content: "different bytes"},
		},
	}
	if _, err := classifyEnvelopeSchema(cache, env); err == nil {
		t.Error("expected error on definition conflict")
	}
}

// TestEndToEnd_AutoRegisterThenDeclared simulates the full cycle:
// first import auto-registers from inline definitions; second import
// (same module/version) lands as declared because the cache is now
// populated.
func TestEndToEnd_AutoRegisterThenDeclared(t *testing.T) {
	cache := newTestCache(t)
	env := &mubridge.Envelope{
		Schema: mubridge.EnvelopeSchema{Module: "mu/aws", Version: "v1"},
		Definitions: []mubridge.EnvelopeDefFile{
			{Path: "ec2.cue", Content: "package aws\n#EC2Instance: {}\n"},
		},
	}
	statusA, err := classifyEnvelopeSchema(cache, env)
	if err != nil {
		t.Fatal(err)
	}
	if statusA != database.ItemSchemaStatusAutoRegistered {
		t.Errorf("first import: %q want auto_registered", statusA)
	}
	statusB, err := classifyEnvelopeSchema(cache, env)
	if err != nil {
		t.Fatal(err)
	}
	if statusB != database.ItemSchemaStatusDeclared {
		t.Errorf("second import: %q want declared", statusB)
	}
}

// TestEndToEnd_ReclassifyUpgradesUnresolved: an unresolved row gets
// upgraded to declared once the schema arrives in the cache.
func TestEndToEnd_ReclassifyUpgradesUnresolved(t *testing.T) {
	tdir := t.TempDir()
	db, err := database.NewCatalogDB(tdir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cache := newTestCache(t)

	if err := db.AddItemSchema(database.ItemSchema{
		ItemID: "item-xyz", SchemaRef: "mu/aws@v1",
		Status: database.ItemSchemaStatusUnresolved,
	}); err != nil {
		t.Fatal(err)
	}
	if err := cache.Insert("mu/aws", "v1", []muschemas.File{
		{RelPath: "ec2.cue", Content: []byte("package aws\n#EC2Instance: {}\n")},
	}); err != nil {
		t.Fatal(err)
	}
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
		t.Fatal("expected ref to resolve")
	}
	if err := db.AddItemSchema(database.ItemSchema{
		ItemID:    rows[0].ItemID,
		SchemaRef: rows[0].SchemaRef,
		Status:    database.ItemSchemaStatusDeclared,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListItemSchemas("item-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != database.ItemSchemaStatusDeclared {
		t.Errorf("after upgrade: %+v", got)
	}
}
