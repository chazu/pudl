package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	tmpDir := t.TempDir()

	opts := InitOptions{
		Dir:     tmpDir,
		Verbose: false,
	}

	if err := Init(opts); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// .pudl/ should exist
	if _, err := os.Stat(filepath.Join(tmpDir, ".pudl")); err != nil {
		t.Errorf("expected .pudl/ directory: %v", err)
	}

	// .claude/skills/ should exist with skill files
	skillsDir := filepath.Join(tmpDir, ".claude", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("reading .claude/skills/: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected skill directories in .claude/skills/")
	}

	// Each skill dir should have a SKILL.md
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		info, err := os.Stat(skillFile)
		if err != nil {
			t.Errorf("expected SKILL.md in %s: %v", entry.Name(), err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("SKILL.md in %s is empty", entry.Name())
		}
	}
}

func TestInit_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .pudl/ first
	os.MkdirAll(filepath.Join(tmpDir, ".pudl"), 0755)

	opts := InitOptions{
		Dir:     tmpDir,
		Verbose: false,
	}

	err := Init(opts)
	if err == nil {
		t.Fatal("expected error when .pudl/ already exists")
	}
}

func TestInit_ForceOverwrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .pudl/ first
	os.MkdirAll(filepath.Join(tmpDir, ".pudl"), 0755)

	opts := InitOptions{
		Dir:     tmpDir,
		Force:   true,
		Verbose: false,
	}

	if err := Init(opts); err != nil {
		t.Fatalf("Init() with force should succeed: %v", err)
	}
}
