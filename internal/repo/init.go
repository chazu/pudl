package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"pudl/internal/skills"
)

const pudlDirName = ".pudl"

// InitOptions contains options for repo initialization.
type InitOptions struct {
	Dir     string // Directory to initialize (defaults to cwd)
	Force   bool   // Overwrite existing .pudl/ directory
	Verbose bool
}

// Init initializes a .pudl/ directory in the target repo and installs
// Claude skills into .claude/skills/.
func Init(opts InitOptions) error {
	dir := opts.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	}

	pudlDir := filepath.Join(dir, pudlDirName)

	// Check for existing .pudl/
	if !opts.Force {
		if _, err := os.Stat(pudlDir); err == nil {
			return fmt.Errorf("repo already initialized at %s (use --force to reinitialize)", pudlDir)
		}
	}

	// Create .pudl/ directory
	if err := os.MkdirAll(pudlDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", pudlDir, err)
	}

	if opts.Verbose {
		fmt.Printf("Created %s\n", pudlDir)
	}

	// Install skills into .claude/skills/
	claudeSkillsDir := filepath.Join(dir, ".claude", "skills")
	if err := os.MkdirAll(claudeSkillsDir, 0755); err != nil {
		return fmt.Errorf("creating .claude/skills/: %w", err)
	}

	if err := skills.WriteSkills(claudeSkillsDir); err != nil {
		return fmt.Errorf("writing skill files: %w", err)
	}

	skillList, _ := skills.ListSkills()
	if opts.Verbose {
		fmt.Printf("Installed %d PUDL skills to .claude/skills/\n", len(skillList))
	}

	return nil
}
