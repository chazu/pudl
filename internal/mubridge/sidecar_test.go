package mubridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSidecar_Absent(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "out.json")
	os.WriteFile(data, []byte("{}"), 0o644)
	got, err := ReadSidecar(data)
	if err != nil {
		t.Fatalf("ReadSidecar: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil sidecar when absent, got %+v", got)
	}
}

func TestReadSidecar_Present(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "out.json")
	os.WriteFile(data, []byte("{}"), 0o644)
	side := []byte(`{"module":"mu/aws","version":"v1","definition":"#EC2Instance","source":"vendored"}`)
	os.WriteFile(SidecarPath(data), side, 0o644)

	got, err := ReadSidecar(data)
	if err != nil {
		t.Fatalf("ReadSidecar: %v", err)
	}
	if got == nil {
		t.Fatal("expected sidecar, got nil")
	}
	if got.Module != "mu/aws" || got.Version != "v1" || got.Definition != "#EC2Instance" {
		t.Errorf("sidecar fields: %+v", got)
	}
	if got.CanonicalRef() != "mu/aws@v1#EC2Instance" {
		t.Errorf("CanonicalRef = %q, want mu/aws@v1#EC2Instance", got.CanonicalRef())
	}
}

func TestReadSidecar_Malformed(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "out.json")
	os.WriteFile(SidecarPath(data), []byte("{ not json"), 0o644)
	if _, err := ReadSidecar(data); err == nil {
		t.Error("expected error for malformed sidecar")
	}
}

func TestReadSidecar_MissingFields(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "out.json")
	os.WriteFile(SidecarPath(data), []byte(`{"module":"mu/aws"}`), 0o644)
	if _, err := ReadSidecar(data); err == nil {
		t.Error("expected error when version is missing")
	}
}

func TestCanonicalRef_NoDefinition(t *testing.T) {
	s := SchemaSidecar{Module: "mu/aws", Version: "v1"}
	if got := s.CanonicalRef(); got != "mu/aws@v1" {
		t.Errorf("got %q, want mu/aws@v1", got)
	}
}

func TestReadSidecar_WithInlineDefinitions(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "out.json")
	os.WriteFile(data, []byte("{}"), 0o644)
	side := []byte(`{
        "module": "mu/aws",
        "version": "v1",
        "definitions": [
            {"path": "ec2.cue", "content": "package aws\n#EC2Instance: {}\n"},
            {"path": "vpc/vpc.cue", "content": "package vpc\n#VPC: {}\n"}
        ]
    }`)
	os.WriteFile(SidecarPath(data), side, 0o644)

	got, err := ReadSidecar(data)
	if err != nil {
		t.Fatalf("ReadSidecar: %v", err)
	}
	if len(got.Definitions) != 2 {
		t.Fatalf("Definitions len = %d, want 2", len(got.Definitions))
	}
	if got.Definitions[0].Path != "ec2.cue" {
		t.Errorf("Definitions[0].Path = %q", got.Definitions[0].Path)
	}
}
