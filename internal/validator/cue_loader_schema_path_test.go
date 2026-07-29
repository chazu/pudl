package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderUsesSchemaRootRelativePackagePath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cue.mod", "module.cue"), []byte("module: \"pudl.schemas@v0\"\nlanguage: version: \"v0.16.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(root, "pudl", "k8s")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "k8s.cue"), []byte(`package k8s

#Resource: {
	_pudl: {schema_type: "base", resource_type: "k8s.resource"}
	name: string
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewCUEModuleLoader(root)
	modules, err := loader.LoadAllModules()
	if err != nil {
		t.Fatal(err)
	}
	schemas := loader.GetAllSchemas(modules)
	if _, ok := schemas["pudl/k8s.#Resource"]; !ok {
		t.Fatalf("schemas = %v, want pudl/k8s.#Resource", schemas)
	}
}

func TestLoaderIntegrityAllowsComponentOnlyPackages(t *testing.T) {
	root := writeModuleDir(t, "components", "components.cue", `package components

#Tag: {name: string}
`)
	loader := NewCUEModuleLoader(root)
	modules, err := loader.LoadAllModules()
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.ValidateModuleIntegrity(modules); err != nil {
		t.Fatalf("component-only package should be valid: %v", err)
	}
}
