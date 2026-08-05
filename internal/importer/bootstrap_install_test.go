package importer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyBootstrapSchemasInstallsEveryOwnedFile(t *testing.T) {
	dir := t.TempDir()
	if err := CopyBootstrapSchemas(dir); err != nil {
		t.Fatalf("CopyBootstrapSchemas: %v", err)
	}
	required, err := bootstrapSchemaFiles()
	if err != nil {
		t.Fatalf("bootstrapSchemaFiles: %v", err)
	}
	for _, relPath := range required {
		if _, err := os.Stat(filepath.Join(dir, relPath)); err != nil {
			t.Errorf("installed built-in %q: %v", relPath, err)
		}
	}
}

func TestEnsureBasicSchemasRepairsAnyMissingBuiltIn(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cue.mod"), 0755); err != nil {
		t.Fatalf("mkdir cue.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"pudl.schemas@v0\"\nlanguage: version: \"v0.16.0\"\n"), 0644); err != nil {
		t.Fatalf("write module.cue: %v", err)
	}
	if err := CopyBootstrapSchemas(dir); err != nil {
		t.Fatalf("CopyBootstrapSchemas: %v", err)
	}

	// git.cue and rules.cue were both absent from the former handwritten
	// repair checklist even though fresh initialization installed them.
	for _, relPath := range []string{
		filepath.Join("pudl", "git", "git.cue"),
		filepath.Join("pudl", "rules", "rules.cue"),
	} {
		path := filepath.Join(dir, relPath)
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", relPath, err)
		}
		imp := &EnhancedImporter{schemaPath: dir}
		if err := imp.ensureBasicSchemas(); err != nil {
			t.Fatalf("ensureBasicSchemas after removing %s: %v", relPath, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("repaired built-in %q: %v", relPath, err)
		}
	}
}
