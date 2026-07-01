package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestEmbeddedSkillsInSync guards against the embedded files/*.md copies (what
// `pudl init` writes) drifting from the canonical skills/<name>/SKILL.md
// sources. If this fails, run `go generate ./internal/skills` and commit.
func TestEmbeddedSkillsInSync(t *testing.T) {
	skills, err := ListSkills()
	if err != nil {
		t.Fatalf("ListSkills() error: %v", err)
	}

	for _, s := range skills {
		embedded, err := skillFiles.ReadFile("files/" + s.Filename)
		if err != nil {
			t.Errorf("reading embedded %q: %v", s.Filename, err)
			continue
		}
		src := filepath.Join("..", "..", "skills", s.Name, "SKILL.md")
		source, err := os.ReadFile(src)
		if err != nil {
			t.Errorf("reading source %q: %v", src, err)
			continue
		}
		if !bytes.Equal(embedded, source) {
			t.Errorf("embedded skill %q is out of sync with %s; run `go generate ./internal/skills`", s.Name, src)
		}
	}
}

func TestListSkills(t *testing.T) {
	skills, err := ListSkills()
	if err != nil {
		t.Fatalf("ListSkills() error: %v", err)
	}

	if len(skills) == 0 {
		t.Fatal("expected at least one skill")
	}

	// Verify known skill names exist
	names := make(map[string]bool)
	for _, s := range skills {
		names[s.Name] = true
	}

	expected := []string{"pudl-core"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected skill %q not found", name)
		}
	}
}

func TestReadSkill(t *testing.T) {
	content, err := ReadSkill("pudl-core")
	if err != nil {
		t.Fatalf("ReadSkill() error: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty skill content")
	}
}

func TestReadSkill_NotFound(t *testing.T) {
	_, err := ReadSkill("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}
}

func TestWriteSkills(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "skills-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := WriteSkills(tmpDir); err != nil {
		t.Fatalf("WriteSkills() error: %v", err)
	}

	// Verify skill directories and files were created
	expected := []string{"pudl-core"}
	for _, name := range expected {
		skillPath := filepath.Join(tmpDir, name, "SKILL.md")
		info, err := os.Stat(skillPath)
		if err != nil {
			t.Errorf("expected skill file %q: %v", skillPath, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("skill file %q is empty", skillPath)
		}
	}
}
