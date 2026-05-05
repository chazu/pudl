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

func TestClassifySidecar_Unresolved(t *testing.T) {
	cache := newTestCache(t)
	side := &mubridge.SchemaSidecar{Module: "mu/aws", Version: "v1"}
	status, err := classifySidecar(cache, side)
	if err != nil {
		t.Fatalf("classifySidecar: %v", err)
	}
	if status != database.ItemSchemaStatusUnresolved {
		t.Errorf("status = %q, want unresolved", status)
	}
}

func TestClassifySidecar_AutoRegistered(t *testing.T) {
	cache := newTestCache(t)
	side := &mubridge.SchemaSidecar{
		Module: "mu/aws", Version: "v1",
		Definitions: []mubridge.SidecarDefinitionFile{
			{Path: "ec2.cue", Content: "package aws\n#EC2Instance: {}\n"},
		},
	}
	status, err := classifySidecar(cache, side)
	if err != nil {
		t.Fatalf("classifySidecar: %v", err)
	}
	if status != database.ItemSchemaStatusAutoRegistered {
		t.Errorf("status = %q, want auto_registered", status)
	}
	if !cache.Has("mu/aws", "v1") {
		t.Error("expected cache.Has after auto-register")
	}
}

func TestClassifySidecar_Declared(t *testing.T) {
	cache := newTestCache(t)
	// Pre-populate cache.
	if err := cache.Insert("mu/aws", "v1", []muschemas.File{
		{RelPath: "ec2.cue", Content: []byte("package aws\n#EC2Instance: {}\n")},
	}); err != nil {
		t.Fatal(err)
	}
	side := &mubridge.SchemaSidecar{Module: "mu/aws", Version: "v1"}
	status, err := classifySidecar(cache, side)
	if err != nil {
		t.Fatalf("classifySidecar: %v", err)
	}
	if status != database.ItemSchemaStatusDeclared {
		t.Errorf("status = %q, want declared", status)
	}
}

func TestClassifySidecar_DeclaredWithDefinitionsMatching(t *testing.T) {
	cache := newTestCache(t)
	cuesrc := []byte("package aws\n#EC2Instance: {}\n")
	if err := cache.Insert("mu/aws", "v1", []muschemas.File{{RelPath: "ec2.cue", Content: cuesrc}}); err != nil {
		t.Fatal(err)
	}
	side := &mubridge.SchemaSidecar{
		Module: "mu/aws", Version: "v1",
		Definitions: []mubridge.SidecarDefinitionFile{{Path: "ec2.cue", Content: string(cuesrc)}},
	}
	status, err := classifySidecar(cache, side)
	if err != nil {
		t.Fatalf("classifySidecar (matching): %v", err)
	}
	if status != database.ItemSchemaStatusDeclared {
		t.Errorf("status = %q, want declared", status)
	}
}

func TestClassifySidecar_DeclaredWithDefinitionsConflict(t *testing.T) {
	cache := newTestCache(t)
	if err := cache.Insert("mu/aws", "v1", []muschemas.File{
		{RelPath: "ec2.cue", Content: []byte("package aws\n#EC2Instance: {}\n")},
	}); err != nil {
		t.Fatal(err)
	}
	side := &mubridge.SchemaSidecar{
		Module: "mu/aws", Version: "v1",
		Definitions: []mubridge.SidecarDefinitionFile{
			{Path: "ec2.cue", Content: "different content"},
		},
	}
	if _, err := classifySidecar(cache, side); err == nil {
		t.Error("expected error on definition conflict")
	}
}
