package factstore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pudl/pkg/factstore"
)

func TestGlobalDir(t *testing.T) {
	if factstore.GlobalDir() == "" {
		t.Fatal("GlobalDir should not be empty")
	}
}

// Outside any workspace, RepoDir is empty and RulePaths has only the global path.
func TestDiscoverWorkspaceGlobalOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pudl-resolve-global-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ws, err := factstore.DiscoverWorkspace(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.RepoDir != "" {
		t.Errorf("expected empty RepoDir outside a workspace, got %q", ws.RepoDir)
	}
	if len(ws.RulePaths) != 1 {
		t.Fatalf("expected 1 rule path (global), got %d", len(ws.RulePaths))
	}
	if !strings.HasPrefix(ws.RulePaths[0], ws.GlobalDir) {
		t.Errorf("rule path %q should be under global dir %q", ws.RulePaths[0], ws.GlobalDir)
	}
}

// Inside a workspace, RepoDir is set and repo rules come last (highest priority).
func TestDiscoverWorkspaceRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pudl-resolve-repo-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pudlDir := filepath.Join(tmpDir, ".pudl")
	if err := os.MkdirAll(pudlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pudlDir, "workspace.cue"), []byte(`name: "testws"`), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, err := factstore.DiscoverWorkspace(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.RepoDir != pudlDir {
		t.Errorf("expected RepoDir %q, got %q", pudlDir, ws.RepoDir)
	}
	if len(ws.RulePaths) != 2 {
		t.Fatalf("expected 2 rule paths (global, repo), got %d", len(ws.RulePaths))
	}
	// Repo rules must be last so they shadow global rules.
	if !strings.HasPrefix(ws.RulePaths[1], pudlDir) {
		t.Errorf("repo rule path should come last, got %v", ws.RulePaths)
	}
}
