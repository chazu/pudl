package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigCanBeScopedToRepositoryPudlDir(t *testing.T) {
	pudlDir := filepath.Join(t.TempDir(), ".pudl")
	cfg := DefaultConfigFor(pudlDir)
	if err := cfg.SaveTo(pudlDir); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFrom(pudlDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaPath != filepath.Join(pudlDir, "schema") {
		t.Fatalf("SchemaPath = %q", loaded.SchemaPath)
	}
	if loaded.DataPath != filepath.Join(pudlDir, "data") {
		t.Fatalf("DataPath = %q", loaded.DataPath)
	}
	if _, err := os.Stat(ConfigPath(pudlDir)); err != nil {
		t.Fatalf("local config was not saved: %v", err)
	}
}

func TestSetAndResetConfigValueAtDoNotTouchGlobalConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pudlDir := filepath.Join(t.TempDir(), ".pudl")
	customData := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(customData, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SetConfigValueAt(pudlDir, "data_path", customData); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(GetConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("global config unexpectedly created: %v", err)
	}
	if err := ResetToDefaultsAt(pudlDir); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(pudlDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataPath != filepath.Join(pudlDir, "data") {
		t.Fatalf("reset DataPath = %q", cfg.DataPath)
	}
}
