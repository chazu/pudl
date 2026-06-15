package validator

import (
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
)

// writeModuleDir scaffolds a minimal CUE module rooted at dir with a single
// package file, returning the root so it can be handed to NewCUEModuleLoader.
func writeModuleDir(t *testing.T, pkgName, fileName, content string) string {
	t.Helper()
	root := t.TempDir()

	modDir := filepath.Join(root, "cue.mod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("failed to create cue.mod: %v", err)
	}
	moduleContent := "module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"
	if err := os.WriteFile(filepath.Join(modDir, "module.cue"), []byte(moduleContent), 0644); err != nil {
		t.Fatalf("failed to write module.cue: %v", err)
	}

	pkgDir := filepath.Join(root, pkgName)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create package dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, fileName), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", fileName, err)
	}
	return root
}

// TestComponentSchemaBoundary verifies D1: a definition with a `_pudl` block is
// registered as a schema; a definition without one is a component and is skipped;
// a list-type definition without `_pudl` is still registered (collections cannot
// carry metadata since arrays have no fields).
func TestComponentSchemaBoundary(t *testing.T) {
	content := `package git

// component -- no _pudl, must NOT be registered as a schema.
#GitRemote: {
	name: string
	url:  string
}

// tracked schema -- has _pudl, must be registered.
#GitRepository: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "git.repository"
		identity_fields: ["name"]
		tracked_fields:  ["default_branch"]
	}
	name:           string
	default_branch: string
	remotes: [...#GitRemote]
}

// list-type schema -- no _pudl (arrays have no fields), must still register.
#RepoList: [...#GitRepository]
`
	root := writeModuleDir(t, "git", "git.cue", content)

	loader := NewCUEModuleLoader(root)
	modules, err := loader.LoadAllModules()
	if err != nil {
		t.Fatalf("LoadAllModules failed: %v", err)
	}

	schemas := loader.GetAllSchemas(modules)
	metadata := loader.GetAllMetadata(modules)

	const (
		repo      = "git.#GitRepository"
		component = "git.#GitRemote"
		listType  = "git.#RepoList"
	)

	if _, ok := schemas[repo]; !ok {
		t.Errorf("expected schema %q to be registered, got schemas: %v", repo, keys(schemas))
	}
	if _, ok := schemas[component]; ok {
		t.Errorf("component %q (no _pudl) must not be registered as a schema", component)
	}
	if _, ok := schemas[listType]; !ok {
		t.Errorf("list-type schema %q must remain registered, got schemas: %v", listType, keys(schemas))
	}

	// Metadata must track the same set as schemas (no phantom component metadata).
	if _, ok := metadata[component]; ok {
		t.Errorf("component %q must not have registered metadata", component)
	}
	if md, ok := metadata[listType]; !ok || !md.IsListType {
		t.Errorf("list-type schema %q must be registered with IsListType=true (got ok=%v, meta=%+v)", listType, ok, md)
	}
}

func keys(m map[string]cue.Value) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
