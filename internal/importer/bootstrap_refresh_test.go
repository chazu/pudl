package importer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshBootstrapSchemasLeavesUninitializedRepositoryAlone(t *testing.T) {
	dir := t.TempDir()
	refreshed, err := RefreshBootstrapSchemas(dir)
	if err != nil || refreshed {
		t.Fatalf("RefreshBootstrapSchemas = %v, %v; want false, nil", refreshed, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("wrote into an uninitialized repository: %v", entries)
	}
}

func TestRefreshBootstrapSchemasRewritesOnlyStaleBuiltIns(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cue.mod"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"pudl.schemas@v0\"\nlanguage: version: \"v0.16.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CopyBootstrapSchemas(dir); err != nil {
		t.Fatal(err)
	}
	if refreshed, err := RefreshBootstrapSchemas(dir); err != nil || refreshed {
		t.Fatalf("current built-ins: RefreshBootstrapSchemas = %v, %v; want false, nil", refreshed, err)
	}

	// An older pudl's workspace: no stamp, and a built-in it never shipped.
	vocab := filepath.Join(dir, "pudl", "rules", "graph_vocabulary.cue")
	for _, p := range []string{vocab, filepath.Join(dir, filepath.FromSlash(bootstrapStampFile))} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	refreshed, err := RefreshBootstrapSchemas(dir)
	if err != nil || !refreshed {
		t.Fatalf("stale built-ins: RefreshBootstrapSchemas = %v, %v; want true, nil", refreshed, err)
	}
	if _, err := os.Stat(vocab); err != nil {
		t.Errorf("refresh did not restore graph_vocabulary.cue: %v", err)
	}
	if !bootstrapCurrent(dir) {
		t.Error("refresh did not stamp the built-ins as current")
	}
}
