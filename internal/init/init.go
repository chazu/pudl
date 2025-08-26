package init

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"pudl/internal/config"
)

// InitOptions contains options for initialization
type InitOptions struct {
	Force   bool // Force re-initialization even if already exists
	Verbose bool // Show verbose output
}

// Initialize sets up the PUDL workspace
func Initialize(opts InitOptions) error {
	pudlDir := config.GetPudlDir()
	
	// Check if already initialized (unless force is specified)
	if !opts.Force && config.Exists() {
		if opts.Verbose {
			fmt.Printf("PUDL workspace already initialized at %s\n", pudlDir)
		}
		return nil
	}
	
	if opts.Verbose {
		fmt.Printf("Initializing PUDL workspace at %s\n", pudlDir)
	}
	
	// Create the main PUDL directory
	if err := os.MkdirAll(pudlDir, 0755); err != nil {
		return fmt.Errorf("failed to create PUDL directory: %w", err)
	}
	
	// Load default configuration
	cfg := config.DefaultConfig()
	
	// Create schema directory
	if err := os.MkdirAll(cfg.SchemaPath, 0755); err != nil {
		return fmt.Errorf("failed to create schema directory: %w", err)
	}
	
	// Create data directory
	if err := os.MkdirAll(cfg.DataPath, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	
	// Initialize git repository in schema directory
	if err := initGitRepo(cfg.SchemaPath, opts.Verbose); err != nil {
		return fmt.Errorf("failed to initialize git repository: %w", err)
	}
	
	// Save configuration
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}
	
	if opts.Verbose {
		fmt.Println("✅ PUDL workspace initialized successfully!")
		fmt.Printf("   Schema repository: %s\n", cfg.SchemaPath)
		fmt.Printf("   Data directory: %s\n", cfg.DataPath)
		fmt.Printf("   Configuration: %s\n", config.GetConfigPath())
	}
	
	return nil
}

// initGitRepo initializes a git repository in the specified directory
func initGitRepo(dir string, verbose bool) error {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		if verbose {
			fmt.Println("⚠️  Git not found - schema repository will not be version controlled")
		}
		return nil // Not a fatal error
	}
	
	// Check if already a git repository
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		if verbose {
			fmt.Printf("Git repository already exists in %s\n", dir)
		}
		return nil
	}
	
	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run git init: %w", err)
	}
	
	// Create initial .gitignore
	gitignorePath := filepath.Join(dir, ".gitignore")
	gitignoreContent := `# PUDL Schema Repository
# Add any files you don't want to version control here

# OS generated files
.DS_Store
.DS_Store?
._*
.Spotlight-V100
.Trashes
ehthumbs.db
Thumbs.db

# Editor files
*.swp
*.swo
*~
.vscode/
.idea/
`
	
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}
	
	// Create initial README
	readmePath := filepath.Join(dir, "README.md")
	readmeContent := `# PUDL Schema Repository

This directory contains CUE schema definitions for your PUDL data lake.

## Structure

- Place your schema files (*.cue) in this directory
- Use subdirectories to organize schemas by type or source
- All changes are version controlled with git

## Usage

- Add new schemas: ` + "`pudl schema add <name> <cue-file>`" + `
- List schemas: ` + "`pudl schema list`" + `
- Commit changes: ` + "`pudl schema commit -m \"message\"`" + `

## Getting Started

Create your first schema by importing some data:

` + "```bash" + `
# Import data and let PUDL infer a schema
pudl import --path data.json --infer-schema

# Review and confirm the inferred schema
pudl schema review
` + "```" + `
`
	
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("failed to create README.md: %w", err)
	}
	
	// Add and commit initial files
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = dir
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("failed to add files to git: %w", err)
	}
	
	commitCmd := exec.Command("git", "commit", "-m", "Initial PUDL schema repository")
	commitCmd.Dir = dir
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}
	
	if verbose {
		fmt.Printf("✅ Git repository initialized in %s\n", dir)
	}
	
	return nil
}

// AutoInitialize performs automatic initialization if needed
func AutoInitialize() error {
	if config.Exists() {
		return nil // Already initialized
	}
	
	// Perform silent initialization
	return Initialize(InitOptions{
		Force:   false,
		Verbose: false,
	})
}
