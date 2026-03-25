package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
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

func TestInit_CreatesWorkspaceCue(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "my-project")
	os.MkdirAll(tmpDir, 0755)

	opts := InitOptions{Dir: tmpDir}
	if err := Init(opts); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	cuePath := filepath.Join(tmpDir, ".pudl", "workspace.cue")
	data, err := os.ReadFile(cuePath)
	if err != nil {
		t.Fatalf("expected workspace.cue to exist: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `name: "my-project"`) {
		t.Errorf("workspace.cue should contain directory name, got:\n%s", content)
	}
}

func TestInit_CreatesSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	opts := InitOptions{Dir: tmpDir}
	if err := Init(opts); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	for _, sub := range []string{"schema", "definitions"} {
		subDir := filepath.Join(tmpDir, ".pudl", sub)
		info, err := os.Stat(subDir)
		if err != nil {
			t.Errorf("expected %s/ to exist: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", sub)
		}

		// Check .gitkeep exists
		gitkeep := filepath.Join(subDir, ".gitkeep")
		if _, err := os.Stat(gitkeep); err != nil {
			t.Errorf("expected .gitkeep in %s/: %v", sub, err)
		}
	}
}

func TestInit_Force_OverwritesWorkspaceCue(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "test-proj")
	os.MkdirAll(tmpDir, 0755)

	// First init
	if err := Init(InitOptions{Dir: tmpDir}); err != nil {
		t.Fatalf("first Init() error: %v", err)
	}

	// Modify workspace.cue
	cuePath := filepath.Join(tmpDir, ".pudl", "workspace.cue")
	os.WriteFile(cuePath, []byte(`name: "modified"`), 0644)

	// Second init with force
	if err := Init(InitOptions{Dir: tmpDir, Force: true}); err != nil {
		t.Fatalf("second Init() error: %v", err)
	}

	data, err := os.ReadFile(cuePath)
	if err != nil {
		t.Fatalf("reading workspace.cue: %v", err)
	}

	if strings.Contains(string(data), "modified") {
		t.Error("expected workspace.cue to be overwritten with --force")
	}
	if !strings.Contains(string(data), `name: "test-proj"`) {
		t.Error("expected workspace.cue to contain original directory name after force")
	}
}

func TestInit_NoForce_PreservesWorkspaceCue(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "test-proj")
	os.MkdirAll(tmpDir, 0755)

	// First init
	if err := Init(InitOptions{Dir: tmpDir}); err != nil {
		t.Fatalf("first Init() error: %v", err)
	}

	// Modify workspace.cue
	cuePath := filepath.Join(tmpDir, ".pudl", "workspace.cue")
	customContent := `name: "custom-name"`
	os.WriteFile(cuePath, []byte(customContent), 0644)

	// Second init without force — should fail because .pudl/ exists
	// But workspace.cue should be preserved
	_ = Init(InitOptions{Dir: tmpDir})

	data, err := os.ReadFile(cuePath)
	if err != nil {
		t.Fatalf("reading workspace.cue: %v", err)
	}

	if string(data) != customContent {
		t.Errorf("expected workspace.cue to be preserved, got:\n%s", string(data))
	}
}

func TestInit_WorkspaceCue_ValidCUE(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "cue-test")
	os.MkdirAll(tmpDir, 0755)

	if err := Init(InitOptions{Dir: tmpDir}); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	cuePath := filepath.Join(tmpDir, ".pudl", "workspace.cue")
	data, err := os.ReadFile(cuePath)
	if err != nil {
		t.Fatalf("reading workspace.cue: %v", err)
	}

	ctx := cuecontext.New()
	val := ctx.CompileBytes(data)
	if val.Err() != nil {
		t.Fatalf("workspace.cue is not valid CUE: %v", val.Err())
	}
}
