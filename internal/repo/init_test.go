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

func TestInit_AlreadyExistsRepairsWithoutFailing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .pudl/ first
	os.MkdirAll(filepath.Join(tmpDir, ".pudl"), 0755)

	opts := InitOptions{
		Dir:     tmpDir,
		Verbose: false,
	}

	if err := Init(opts); err != nil {
		t.Fatalf("Init() should repair an existing .pudl: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".pudl", "workspace.cue")); err != nil {
		t.Fatalf("workspace marker was not repaired: %v", err)
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

	for _, sub := range []string{"schema/models", "definitions", "populators"} {
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

	for _, sub := range []string{"data/raw", "data/metadata", "data/sqlite"} {
		info, err := os.Stat(filepath.Join(tmpDir, ".pudl", sub))
		if err != nil {
			t.Errorf("expected %s/ to exist: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", sub)
		}
	}

	for _, path := range []string{
		".pudl/.gitignore",
		".pudl/config.yaml",
		".pudl/schema/cue.mod/module.cue",
		".pudl/schema/pudl/systemmodel/systemmodel.cue",
		".pudl/schema/pudl/rules/rules.cue",
	} {
		if _, err := os.Stat(filepath.Join(tmpDir, path)); err != nil {
			t.Errorf("expected initialized file %s: %v", path, err)
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

	// Second init without force repairs owned files but preserves workspace.cue.
	if err := Init(InitOptions{Dir: tmpDir}); err != nil {
		t.Fatalf("second Init() error: %v", err)
	}

	data, err := os.ReadFile(cuePath)
	if err != nil {
		t.Fatalf("reading workspace.cue: %v", err)
	}

	if string(data) != customContent {
		t.Errorf("expected workspace.cue to be preserved, got:\n%s", string(data))
	}
}

func TestInit_NoForce_PreservesLocalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	if err := Init(InitOptions{Dir: tmpDir}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tmpDir, ".pudl", "config.yaml")
	const custom = "schema_path: /custom/schema\ndata_path: /custom/data\nversion: custom\n"
	if err := os.WriteFile(configPath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Init(InitOptions{Dir: tmpDir}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != custom {
		t.Fatalf("repo init replaced authored config:\n%s", got)
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
