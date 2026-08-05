package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/workspace"
)

func TestEffectivePudlDirUsesRepositoryWorkspace(t *testing.T) {
	root := t.TempDir()
	pudlDir := filepath.Join(root, ".pudl")
	if err := os.MkdirAll(pudlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pudlDir, "workspace.cue"), []byte(`name: "local"`), 0o644); err != nil {
		t.Fatal(err)
	}

	policy, err := workspace.Resolve(root, filepath.Join(t.TempDir(), "global"))
	if err != nil {
		t.Fatal(err)
	}
	previous := wsPolicy
	wsPolicy = policy
	t.Cleanup(func() { wsPolicy = previous })

	if got := effectivePudlDir(); got != pudlDir {
		t.Fatalf("effectivePudlDir() = %q, want %q", got, pudlDir)
	}
	cfg, err := loadEffectiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SchemaPath != filepath.Join(pudlDir, "schema") {
		t.Fatalf("SchemaPath = %q", cfg.SchemaPath)
	}
	if cfg.DataPath != filepath.Join(pudlDir, "data") {
		t.Fatalf("DataPath = %q", cfg.DataPath)
	}
}

func TestEffectivePudlDirFallsBackToGlobal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	previous := wsPolicy
	wsPolicy = nil
	t.Cleanup(func() { wsPolicy = previous })

	if got, want := effectivePudlDir(), config.GetPudlDir(); got != want {
		t.Fatalf("effectivePudlDir() = %q, want %q", got, want)
	}
}

func TestEffectiveConfigRejectsRepositoryPathEscape(t *testing.T) {
	root := t.TempDir()
	pudlDir := filepath.Join(root, ".pudl")
	if err := os.MkdirAll(pudlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pudlDir, "workspace.cue"), []byte(`name: "local"`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfigFor(pudlDir)
	cfg.DataPath = t.TempDir()
	if err := cfg.SaveTo(pudlDir); err != nil {
		t.Fatal(err)
	}
	policy, err := workspace.Resolve(root, filepath.Join(t.TempDir(), "global"))
	if err != nil {
		t.Fatal(err)
	}
	previous := wsPolicy
	wsPolicy = policy
	t.Cleanup(func() { wsPolicy = previous })

	if _, err := loadEffectiveConfig(); err == nil {
		t.Fatal("expected repository data_path escape to be rejected")
	}
}
