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
	
	// Initialize CUE module in schema directory
	if err := initCUEModule(cfg.SchemaPath, opts.Verbose); err != nil {
		return fmt.Errorf("failed to initialize CUE module: %w", err)
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

This directory contains CUE schema definitions for your PUDL data lake, organized as a proper CUE module with third-party dependencies.

## Structure

- **cue.mod/module.cue** - Module definition with third-party dependencies
- **pudl/** - Your local schema definitions
- **examples/** - Usage examples showing how to combine local and third-party schemas
- All changes are version controlled with git

## Third-Party Modules

This repository includes access to curated third-party schemas:

- **cue.dev/x/k8s.io** - Complete Kubernetes API schemas for all resource types (included by default)
- Additional modules can be added using ` + "`pudl module add <module>`" + ` or ` + "`cue mod get <module>`" + `

## Usage

### Schema Management
- Add new schemas: ` + "`pudl schema add <name> <cue-file>`" + `
- List schemas: ` + "`pudl schema list`" + `
- Commit changes: ` + "`pudl schema commit -m \"message\"`" + `

### Module Management
- Add dependencies: ` + "`cue mod tidy`" + ` (or ` + "`pudl module add <module>`" + ` when available)
- Update dependencies: ` + "`cue mod tidy`" + `

## Getting Started

### Import Data with Schema Inference
` + "```bash" + `
# Import data and let PUDL infer a schema
pudl import --path data.json --infer-schema

# Review and confirm the inferred schema
pudl schema review
` + "```" + `

### Use Third-Party Schemas
` + "```cue" + `
package myschema

import k8s "cue.dev/x/k8s.io/api/apps/v1"

// Validate Kubernetes deployments with PUDL metadata
#MyDeployment: k8s.#Deployment & {
    _pudl: {
        schema_type: "kubernetes"
        resource_type: "deployment"
    }
}
` + "```" + `

See the **examples/** directory for more usage patterns.
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

// initCUEModule initializes a CUE module in the schema directory
func initCUEModule(schemaDir string, verbose bool) error {
	// Check if cue command is available
	if _, err := exec.LookPath("cue"); err != nil {
		if verbose {
			fmt.Println("⚠️  CUE command not found - CUE module will not be initialized")
			fmt.Println("   Install CUE from https://cuelang.org/docs/install/ to enable third-party module support")
		}
		return nil // Not a fatal error, but functionality will be limited
	}

	// Create cue.mod directory
	cueModDir := filepath.Join(schemaDir, "cue.mod")
	if err := os.MkdirAll(cueModDir, 0755); err != nil {
		return fmt.Errorf("failed to create cue.mod directory: %w", err)
	}

	// Check if module.cue already exists
	moduleCuePath := filepath.Join(cueModDir, "module.cue")
	if _, err := os.Stat(moduleCuePath); err == nil {
		if verbose {
			fmt.Printf("CUE module already exists in %s\n", schemaDir)
		}
		return nil
	}

	// Create module.cue file with Kubernetes schemas dependency
	moduleContent := `language: version: "v0.14.0"

module: "pudl.schemas@v0"

// Third-party dependencies for comprehensive schema support
deps: {
    "cue.dev/x/k8s.io@v0": v: "v0.1.0"
}

source: kind: "self"

description: "PUDL Schema Repository - CUE schemas for data lake validation and processing with Kubernetes API support"
`

	if err := os.WriteFile(moduleCuePath, []byte(moduleContent), 0644); err != nil {
		return fmt.Errorf("failed to create module.cue: %w", err)
	}

	// Run cue mod tidy to fetch dependencies
	if verbose {
		fmt.Println("Fetching CUE module dependencies...")
	}

	tidyCmd := exec.Command("cue", "mod", "tidy")
	tidyCmd.Dir = schemaDir
	if output, err := tidyCmd.CombinedOutput(); err != nil {
		if verbose {
			fmt.Printf("⚠️  Failed to fetch CUE dependencies: %s\n", string(output))
			fmt.Println("   You can run 'cue mod tidy' manually later in the schema directory")
		}
		// Don't return error - module structure is still valid
	} else if verbose {
		fmt.Println("✅ CUE module dependencies fetched successfully")
	}

	// Create local schema directory structure
	localSchemaDir := filepath.Join(schemaDir, "pudl")
	if err := os.MkdirAll(localSchemaDir, 0755); err != nil {
		return fmt.Errorf("failed to create local schema directory: %w", err)
	}

	// Create examples directory
	examplesDir := filepath.Join(schemaDir, "examples")
	if err := os.MkdirAll(examplesDir, 0755); err != nil {
		return fmt.Errorf("failed to create examples directory: %w", err)
	}

	// Create example usage file
	exampleContent := `package examples

// Example 1: Local PUDL schema definition
#BasicKubernetesDeployment: {
	// PUDL metadata for tracking and validation
	_pudl: {
		schema_type:      "kubernetes"
		resource_type:    "deployment"
		cascade_priority: 20
		identity_fields: ["metadata.name", "metadata.namespace"]
		tracked_fields:  ["spec.replicas", "spec.template.spec.containers"]
		compliance_level: "strict"
	}

	// Basic Kubernetes Deployment structure
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      string
		namespace: string | *"default"
		labels?: [string]: string
	}
	spec: {
		replicas: int & >=1
		selector: matchLabels: [string]: string
		template: {
			metadata: labels: [string]: string
			spec: {
				containers: [...{
					name:  string
					image: string
					ports?: [...{
						containerPort: int
						protocol?:     "TCP" | "UDP"
					}]
				}]
			}
		}
	}
}

// Example 2: Using the local schema
exampleDeployment: #BasicKubernetesDeployment & {
	metadata: {
		name: "example-app"
		labels: app: "example"
	}
	spec: {
		replicas: 3
		selector: matchLabels: app: "example"
		template: {
			metadata: labels: app: "example"
			spec: containers: [{
				name:  "app"
				image: "nginx:latest"
				ports: [{
					containerPort: 80
					protocol:      "TCP"
				}]
			}]
		}
	}
}

// Example 3: Using official Kubernetes schemas from cue.dev/x/k8s.io
import (
    apps "cue.dev/x/k8s.io/api/apps/v1"
    core "cue.dev/x/k8s.io/api/core/v1"
)

// Extend official Kubernetes Deployment with PUDL metadata
#KubernetesDeployment: apps.#Deployment & {
    _pudl: {
        schema_type: "kubernetes"
        resource_type: "k8s.apps.deployment"
        cascade_priority: 95
        identity_fields: ["metadata.name", "metadata.namespace"]
        tracked_fields: ["spec.replicas", "status.readyReplicas"]
        compliance_level: "strict"
    }
}

// Extend official Kubernetes Pod with PUDL metadata
#KubernetesPod: core.#Pod & {
    _pudl: {
        schema_type: "kubernetes"
        resource_type: "k8s.core.pod"
        cascade_priority: 95
        identity_fields: ["metadata.name", "metadata.namespace"]
        tracked_fields: ["status.phase", "spec.containers"]
        compliance_level: "strict"
    }
}

// Extend official Kubernetes Service with PUDL metadata
#KubernetesService: core.#Service & {
    _pudl: {
        schema_type: "kubernetes"
        resource_type: "k8s.core.service"
        cascade_priority: 95
        identity_fields: ["metadata.name", "metadata.namespace"]
        tracked_fields: ["spec.type", "spec.ports", "spec.selector"]
        compliance_level: "strict"
    }
}
`

	examplePath := filepath.Join(examplesDir, "kubernetes.cue")
	if err := os.WriteFile(examplePath, []byte(exampleContent), 0644); err != nil {
		return fmt.Errorf("failed to create example file: %w", err)
	}

	if verbose {
		fmt.Printf("✅ CUE module initialized in %s\n", schemaDir)
		fmt.Println("   - Third-party dependencies: cue.dev/x/k8s.io (complete Kubernetes API schemas)")
		fmt.Println("   - Local schemas: pudl/ (AWS, custom schemas)")
		fmt.Println("   - Examples: examples/ (usage patterns and integrations)")
		fmt.Println("   - Run 'pudl module tidy' to fetch dependencies")
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
