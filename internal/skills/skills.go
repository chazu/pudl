package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed files/*.md
var skillFiles embed.FS

// SkillInfo describes an embedded skill file.
type SkillInfo struct {
	Name     string // e.g., "pudl-core"
	Filename string // e.g., "pudl-core.md"
}

// ListSkills returns metadata for all embedded skill files.
func ListSkills() ([]SkillInfo, error) {
	entries, err := fs.ReadDir(skillFiles, "files")
	if err != nil {
		return nil, fmt.Errorf("reading embedded skill files: %w", err)
	}

	var skills []SkillInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		skills = append(skills, SkillInfo{
			Name:     name,
			Filename: entry.Name(),
		})
	}
	return skills, nil
}

// ReadSkill returns the content of a skill file by name.
func ReadSkill(name string) ([]byte, error) {
	return skillFiles.ReadFile("files/" + name + ".md")
}

// WriteSkills writes all embedded skill files to the target directory.
// Each skill is written as <targetDir>/<skill-name>/SKILL.md.
func WriteSkills(targetDir string) error {
	skills, err := ListSkills()
	if err != nil {
		return err
	}

	for _, skill := range skills {
		content, err := skillFiles.ReadFile("files/" + skill.Filename)
		if err != nil {
			return fmt.Errorf("reading skill %q: %w", skill.Name, err)
		}

		skillDir := filepath.Join(targetDir, skill.Name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			return fmt.Errorf("creating skill directory %q: %w", skillDir, err)
		}

		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, content, 0644); err != nil {
			return fmt.Errorf("writing skill file %q: %w", skillPath, err)
		}
	}

	return nil
}
