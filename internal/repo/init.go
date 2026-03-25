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

	// Create workspace.cue
	workspaceCuePath := filepath.Join(pudlDir, "workspace.cue")
	dirName := filepath.Base(dir)
	if _, err := os.Stat(workspaceCuePath); os.IsNotExist(err) || opts.Force {
		content := generateWorkspaceCue(dirName)
		if err := os.WriteFile(workspaceCuePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("creating workspace.cue: %w", err)
		}
		if opts.Verbose {
			fmt.Printf("Created %s\n", workspaceCuePath)
		}
	}

	// Create schema directory
	schemaDir := filepath.Join(pudlDir, "schema")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		return fmt.Errorf("creating schema/: %w", err)
	}

	// Create definitions directory
	defsDir := filepath.Join(pudlDir, "definitions")
	if err := os.MkdirAll(defsDir, 0755); err != nil {
		return fmt.Errorf("creating definitions/: %w", err)
	}

	// Create .gitkeep in empty directories so git tracks them
	for _, d := range []string{schemaDir, defsDir} {
		gitkeep := filepath.Join(d, ".gitkeep")
		if _, err := os.Stat(gitkeep); os.IsNotExist(err) {
			os.WriteFile(gitkeep, []byte(""), 0644)
		}
	}

	if opts.Verbose {
		fmt.Printf("  workspace.cue  (workspace: %q)\n", dirName)
		fmt.Printf("  schema/        (project-specific CUE schemas)\n")
		fmt.Printf("  definitions/   (desired state definitions)\n")
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

// generateWorkspaceCue returns the content for a new workspace.cue file.
func generateWorkspaceCue(name string) string {
	return fmt.Sprintf(`// PUDL workspace configuration.
// This file marks the root of a per-repo PUDL workspace.

// Workspace name — used as the origin for catalog entries.
// Must be unique across all workspaces sharing the same ~/.pudl/ catalog.
name: %q

// Optional: override toolchain mappings for this workspace.
// These take priority over global config and built-in defaults.
// toolchain_mappings: [
//     {prefix: "myapp", toolchain: "shell"},
// ]
`, name)
}
