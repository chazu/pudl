package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/skills"
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

	// Initialization is deliberately idempotent. Running `pudl repo init`
	// again repairs the owned layout and built-ins while preserving authored
	// workspace configuration unless --force was requested.
	if err := os.MkdirAll(pudlDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", pudlDir, err)
	}

	if opts.Verbose {
		fmt.Printf("Ensured %s\n", pudlDir)
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
	modelsDir := filepath.Join(schemaDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("creating schema/models/: %w", err)
	}
	if err := initLocalCUEModule(schemaDir); err != nil {
		return err
	}
	if err := importer.CopyBootstrapSchemas(schemaDir); err != nil {
		return fmt.Errorf("installing built-in schemas: %w", err)
	}

	// Create definitions directory
	defsDir := filepath.Join(pudlDir, "definitions")
	if err := os.MkdirAll(defsDir, 0755); err != nil {
		return fmt.Errorf("creating definitions/: %w", err)
	}

	// All durable operational state for a repository workspace is local to its
	// .pudl/data tree. The catalog creates sqlite/catalog.db lazily; the other
	// directories are initialized now so commands share one obvious boundary.
	dataDirs := []string{
		filepath.Join(pudlDir, "data"),
		filepath.Join(pudlDir, "data", "raw"),
		filepath.Join(pudlDir, "data", "metadata"),
		filepath.Join(pudlDir, "data", "sqlite"),
	}
	for _, dataDir := range dataDirs {
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return fmt.Errorf("creating local data directory %s: %w", dataDir, err)
		}
	}
	if err := writeLocalGitignore(pudlDir); err != nil {
		return err
	}
	if opts.Force || !config.ExistsAt(pudlDir) {
		if err := config.DefaultConfigFor(pudlDir).SaveTo(pudlDir); err != nil {
			return fmt.Errorf("writing repo-local config: %w", err)
		}
	}

	// Create .gitkeep in empty directories so git tracks them
	for _, d := range []string{modelsDir, defsDir} {
		gitkeep := filepath.Join(d, ".gitkeep")
		if _, err := os.Stat(gitkeep); os.IsNotExist(err) {
			os.WriteFile(gitkeep, []byte(""), 0644)
		}
	}

	if opts.Verbose {
		fmt.Printf("  workspace.cue  (workspace: %q)\n", dirName)
		fmt.Printf("  schema/        (project-specific CUE schemas)\n")
		fmt.Printf("  schema/models/ (registered #SystemModel definitions)\n")
		fmt.Printf("  definitions/   (desired state definitions)\n")
		fmt.Printf("  data/          (repo-local raw data, metadata, and catalog)\n")
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

// Workspace name — used as the origin for catalog entries in this repo's
// .pudl/data/sqlite/catalog.db.
name: %q

// Optional: override toolchain mappings for this workspace.
// These take priority over global config and built-in defaults.
// toolchain_mappings: [
//     {prefix: "myapp", toolchain: "shell"},
// ]

// Optional: restrict provider references that sealed outputs may write.
// Omit for mu compatibility mode; set [] for explicit deny-all.
// secrets: writable_refs: ["pass:myproject/*"]
`, name)
}

func initLocalCUEModule(schemaDir string) error {
	moduleDir := filepath.Join(schemaDir, "cue.mod")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		return fmt.Errorf("creating local CUE module: %w", err)
	}
	modulePath := filepath.Join(moduleDir, "module.cue")
	if _, err := os.Stat(modulePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking local CUE module: %w", err)
	}
	const module = `language: version: "v0.16.0"

module: "pudl.schemas@v0"

source: kind: "self"
`
	if err := os.WriteFile(modulePath, []byte(module), 0o644); err != nil {
		return fmt.Errorf("writing local CUE module: %w", err)
	}
	return nil
}

func writeLocalGitignore(pudlDir string) error {
	path := filepath.Join(pudlDir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking local .gitignore: %w", err)
	}
	const contents = `# Runtime state belongs to this workspace but not to Git.
data/
config.yaml
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("writing local .gitignore: %w", err)
	}
	return nil
}
