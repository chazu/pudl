package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/git"
	"pudl/internal/schema"
)

var (
	schemaVerbose bool
	schemaPackage string
	commitMessage string
	logLimit      int
)

// schemaCmd represents the schema command
var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Manage CUE schemas for data validation",
	Long: `Manage CUE schemas used for data validation and organization in PUDL.

Schemas are organized by packages (aws, k8s, unknown, etc.) and stored in the
schema repository at ~/.pudl/schema/. Each schema is a CUE file that defines
the structure and validation rules for imported data.

Available subcommands:
- list:   Show available schemas organized by package
- add:    Add a new schema file to the repository
- status: Show uncommitted changes in the schema repository
- commit: Commit schema changes to version control
- log:    Show commit history of schema changes

Examples:
    pudl schema list                           # List all schemas
    pudl schema list --package aws             # List schemas in aws package
    pudl schema add aws.rds-instance my.cue    # Add new schema to aws package
    pudl schema status                         # Show uncommitted changes
    pudl schema commit -m "Add RDS schema"     # Commit changes
    pudl schema log                            # Show recent commits`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: show help
		cmd.Help()
	},
}

// schemaListCmd represents the schema list command
var schemaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available schemas",
	Long: `List all available schemas organized by package.

Schemas are displayed with their package, name, file size, and main definition.
Use --verbose for additional details including file paths and metadata information.

Filtering Options:
- --package: Show only schemas from a specific package

Examples:
    pudl schema list                    # List all schemas
    pudl schema list --package aws      # List only AWS schemas
    pudl schema list --verbose          # Show detailed information`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the schema list command and handle any errors
		if err := runSchemaListCommand(); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runSchemaListCommand contains the actual schema list logic with structured error handling
func runSchemaListCommand() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create schema manager
	manager := schema.NewManager(cfg.SchemaPath)

	if schemaPackage != "" {
		// List schemas in specific package
		return listSchemasInPackage(manager, schemaPackage)
	} else {
		// List all schemas
		return listAllSchemas(manager)
	}
}

// schemaAddCmd represents the schema add command
var schemaAddCmd = &cobra.Command{
	Use:   "add <package>.<name> <cue-file>",
	Short: "Add a new schema to the repository",
	Long: `Add a new CUE schema file to the schema repository.

The schema name should be in the format 'package.name' where:
- package: The schema package (aws, k8s, custom, etc.)
- name: The schema name within the package

The CUE file will be validated before adding to ensure:
- Valid CUE syntax
- Proper package declaration
- Required metadata fields (_identity, _tracked, _version)

The schema file will be copied to the appropriate package directory and
added to the git working directory (not automatically committed).

Examples:
    pudl schema add aws.rds-instance rds-schema.cue
    pudl schema add k8s.deployment my-deployment.cue
    pudl schema add custom.api-response api.cue`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the schema add command and handle any errors
		if err := runSchemaAddCommand(args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runSchemaAddCommand contains the actual schema add logic with structured error handling
func runSchemaAddCommand(args []string) error {
	fullSchemaName := args[0]
	sourceFile := args[1]

	// Parse schema name
	packageName, schemaName, err := schema.ParseSchemaName(fullSchemaName)
	if err != nil {
		return errors.NewInputError("Invalid schema name format",
			"Use format: package.schema (e.g., aws.ec2-instance)")
	}

	// Check if source file exists
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		return errors.NewFileNotFoundError(sourceFile)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create schema manager and validator
	manager := schema.NewManager(cfg.SchemaPath)
	validator := schema.NewValidator()

	// Validate the source file first
	fmt.Printf("Validating schema file: %s\n", sourceFile)
	result, err := validator.ValidateSchema(sourceFile)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to validate schema", err)
	}

	// Check validation results
	if !result.Valid {
		return errors.NewCUESyntaxError(sourceFile, fmt.Errorf("validation failed: %v", result.Errors))
	}

	// Show warnings if any
	if len(result.Warnings) > 0 {
		fmt.Println("⚠️  Validation warnings:")
		for _, warning := range result.Warnings {
			fmt.Printf("  - %s\n", warning)
		}
	}

	// Validate package consistency
	if result.PackageName != "" && result.PackageName != packageName {
		return errors.NewInputError(
			fmt.Sprintf("Package mismatch: schema declares package '%s' but adding to package '%s'",
				result.PackageName, packageName),
			"Update the package declaration in the schema file",
			"Use the correct package name in the command")
	}

	// Check if schema already exists
	if manager.SchemaExists(packageName, schemaName) {
		return errors.NewInputError(
			fmt.Sprintf("Schema already exists: %s.%s", packageName, schemaName),
			"Use a different schema name",
			"Remove the existing schema first if you want to replace it")
	}

	// Add the schema
	fmt.Printf("Adding schema: %s.%s\n", packageName, schemaName)
	if err := manager.AddSchema(packageName, schemaName, sourceFile); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to add schema", err)
	}

	fmt.Printf("✅ Schema added successfully: %s.%s\n", packageName, schemaName)

	// Show schema information
	if schemaInfo, err := manager.GetSchema(packageName, schemaName); err == nil {
		fmt.Printf("   Package: %s\n", schemaInfo.Package)
		fmt.Printf("   Name: %s\n", schemaInfo.Name)
		fmt.Printf("   File: %s\n", schemaInfo.FilePath)
		if schemaInfo.Definition != "" {
			fmt.Printf("   Definition: %s\n", schemaInfo.Definition)
		}
		if len(result.Definitions) > 0 {
			fmt.Printf("   Definitions: %s\n", strings.Join(result.Definitions, ", "))
		}
	}

	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Println("   - Review the schema: pudl schema list --package " + packageName)
	fmt.Println("   - Import data using this schema: pudl import --path <file> --schema " + fullSchemaName)
	fmt.Println("   - Commit schema changes: pudl schema commit -m \"Add " + fullSchemaName + " schema\"")

	return nil
}

// schemaStatusCmd represents the schema status command
var schemaStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show uncommitted changes in the schema repository",
	Long: `Show the current status of the schema repository, including:
- Modified files
- Added files
- Deleted files
- Untracked files

This command helps you see what schema changes are pending before committing them.

Examples:
    pudl schema status                  # Show all uncommitted changes
    pudl schema status --verbose        # Show detailed file status`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the schema status command and handle any errors
		if err := runSchemaStatusCommand(); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runSchemaStatusCommand contains the actual schema status logic with structured error handling
func runSchemaStatusCommand() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create git repository instance
	repo := git.NewRepository(cfg.SchemaPath)

	// Validate repository
	if err := repo.ValidateRepository(); err != nil {
		return errors.WrapError(errors.ErrCodeGitOperation, "Schema repository error", err)
	}

	// Get status
	status, err := repo.Status()
	if err != nil {
		return errors.WrapError(errors.ErrCodeGitOperation, "Failed to get repository status", err)
	}

	// Display status
	if status.Clean {
		fmt.Println("✅ Schema repository is clean - no uncommitted changes")
		return nil
	}

	fmt.Println("📋 Schema Repository Status:")
	fmt.Println()

	if len(status.Modified) > 0 {
		fmt.Printf("🔄 Modified files (%d):\n", len(status.Modified))
		for _, file := range status.Modified {
			fmt.Printf("   M  %s\n", file)
		}
		fmt.Println()
	}

	if len(status.Added) > 0 {
		fmt.Printf("➕ Added files (%d):\n", len(status.Added))
		for _, file := range status.Added {
			fmt.Printf("   A  %s\n", file)
		}
		fmt.Println()
	}

	if len(status.Deleted) > 0 {
		fmt.Printf("🗑️  Deleted files (%d):\n", len(status.Deleted))
		for _, file := range status.Deleted {
			fmt.Printf("   D  %s\n", file)
		}
		fmt.Println()
	}

	if len(status.Untracked) > 0 {
		fmt.Printf("❓ Untracked files (%d):\n", len(status.Untracked))
		for _, file := range status.Untracked {
			fmt.Printf("   ?  %s\n", file)
		}
		fmt.Println()
	}

	totalChanges := len(status.Modified) + len(status.Added) + len(status.Deleted) + len(status.Untracked)
	fmt.Printf("Total: %d uncommitted changes\n", totalChanges)
	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Println("   - Commit changes: pudl schema commit -m \"Your commit message\"")
	fmt.Println("   - Review specific files: pudl schema list --verbose")

	return nil
}

// schemaCommitCmd represents the schema commit command
var schemaCommitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit schema changes to version control",
	Long: `Commit all pending schema changes to the git repository.

This command will:
1. Stage all modified, added, and deleted schema files
2. Create a commit with the provided message
3. Update the schema repository history

A commit message is required and should describe the changes being made.

Examples:
    pudl schema commit -m "Add RDS instance schema"
    pudl schema commit -m "Update EC2 schema with new fields"
    pudl schema commit -m "Remove deprecated K8s schemas"`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the schema commit command and handle any errors
		if err := runSchemaCommitCommand(); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runSchemaCommitCommand contains the actual schema commit logic with structured error handling
func runSchemaCommitCommand() error {
	// Validate commit message
	if commitMessage == "" {
		return errors.NewInputError("Commit message is required",
			"Use -m flag to provide a message")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create git repository instance
	repo := git.NewRepository(cfg.SchemaPath)

	// Validate repository
	if err := repo.ValidateRepository(); err != nil {
		return errors.WrapError(errors.ErrCodeGitOperation, "Schema repository error", err)
	}

	// Check if there are changes to commit
	hasChanges, err := repo.HasChanges()
	if err != nil {
		return errors.WrapError(errors.ErrCodeGitOperation, "Failed to check repository status", err)
	}

	if !hasChanges {
		fmt.Println("✅ No changes to commit - schema repository is clean")
		return nil
	}

	// Show what will be committed
	fmt.Println("📋 Committing schema changes...")
	status, err := repo.Status()
	if err != nil {
		return errors.WrapError(errors.ErrCodeGitOperation, "Failed to get repository status", err)
	}

	totalFiles := len(status.Modified) + len(status.Added) + len(status.Deleted) + len(status.Untracked)
	fmt.Printf("   Files to commit: %d\n", totalFiles)

	if schemaVerbose {
		if len(status.Modified) > 0 {
			fmt.Printf("   Modified: %s\n", strings.Join(status.Modified, ", "))
		}
		if len(status.Added) > 0 {
			fmt.Printf("   Added: %s\n", strings.Join(status.Added, ", "))
		}
		if len(status.Deleted) > 0 {
			fmt.Printf("   Deleted: %s\n", strings.Join(status.Deleted, ", "))
		}
		if len(status.Untracked) > 0 {
			fmt.Printf("   Untracked: %s\n", strings.Join(status.Untracked, ", "))
		}
	}

	// Perform the commit
	fmt.Printf("   Message: %s\n", commitMessage)
	fmt.Println()

	if err := repo.AddAndCommit(commitMessage); err != nil {
		return errors.WrapError(errors.ErrCodeGitOperation, "Failed to commit changes", err)
	}

	// Get the new commit info
	lastCommit, err := repo.GetLastCommit()
	if err != nil {
		fmt.Println("✅ Schema changes committed successfully!")
	} else {
		fmt.Printf("✅ Schema changes committed successfully!\n")
		fmt.Printf("   Commit: %s\n", lastCommit.ShortHash)
		fmt.Printf("   Author: %s\n", lastCommit.Author)
		fmt.Printf("   Date: %s\n", lastCommit.Date.Format("2006-01-02 15:04:05"))
	}

	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Println("   - View commit history: pudl schema log")
	fmt.Println("   - Check repository status: pudl schema status")

	return nil
}

// schemaLogCmd represents the schema log command
var schemaLogCmd = &cobra.Command{
	Use:   "log",
	Short: "Show commit history of schema changes",
	Long: `Show the commit history of the schema repository.

This command displays recent commits with information about:
- Commit hash (short and full)
- Author name
- Commit date and time
- Commit message

Use --limit to control how many commits to show.

Examples:
    pudl schema log                     # Show recent commits (default: 10)
    pudl schema log --limit 20          # Show last 20 commits
    pudl schema log --verbose           # Show detailed commit information`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the schema log command and handle any errors
		if err := runSchemaLogCommand(); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runSchemaLogCommand contains the actual schema log logic with structured error handling
func runSchemaLogCommand() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Create git repository instance
	repo := git.NewRepository(cfg.SchemaPath)

	// Validate repository
	if err := repo.ValidateRepository(); err != nil {
		return errors.WrapError(errors.ErrCodeGitOperation, "Schema repository error", err)
	}

	// Get commit history
	commits, err := repo.Log(logLimit)
	if err != nil {
		return errors.WrapError(errors.ErrCodeGitOperation, "Failed to get commit history", err)
	}

	if len(commits) == 0 {
		fmt.Println("No commits found in schema repository")
		fmt.Println()
		fmt.Println("💡 Make your first commit:")
		fmt.Println("   pudl schema commit -m \"Initial schema setup\"")
		return nil
	}

	fmt.Printf("📚 Schema Repository History (%d commits):\n", len(commits))
	fmt.Println()

	for i, commit := range commits {
		if schemaVerbose {
			fmt.Printf("🔸 Commit %d:\n", i+1)
			fmt.Printf("   Hash: %s (%s)\n", commit.Hash, commit.ShortHash)
			fmt.Printf("   Author: %s\n", commit.Author)
			fmt.Printf("   Date: %s\n", commit.Date.Format("2006-01-02 15:04:05 -0700"))
			fmt.Printf("   Message: %s\n", commit.Message)
			fmt.Println()
		} else {
			fmt.Printf("%s %s %s\n",
				commit.ShortHash,
				commit.Date.Format("2006-01-02 15:04"),
				commit.Message)
		}
	}

	if !schemaVerbose {
		fmt.Println()
		fmt.Printf("Showing %d commits. Use --verbose for detailed information.\n", len(commits))
	}

	fmt.Println("💡 Commands:")
	fmt.Println("   - Show repository status: pudl schema status")
	fmt.Println("   - Make new commit: pudl schema commit -m \"message\"")

	return nil
}

func init() {
	rootCmd.AddCommand(schemaCmd)

	// Add subcommands
	schemaCmd.AddCommand(schemaListCmd)
	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaStatusCmd)
	schemaCmd.AddCommand(schemaCommitCmd)
	schemaCmd.AddCommand(schemaLogCmd)

	// Add flags
	schemaListCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show detailed information")
	schemaListCmd.Flags().StringVar(&schemaPackage, "package", "", "Filter by package name")

	// Status command flags
	schemaStatusCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show detailed file status")

	// Commit command flags
	schemaCommitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "Commit message (required)")
	schemaCommitCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show detailed commit information")

	// Log command flags
	schemaLogCmd.Flags().IntVar(&logLimit, "limit", 10, "Number of commits to show")
	schemaLogCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show detailed commit information")
}

// listAllSchemas lists all schemas organized by package
func listAllSchemas(manager *schema.Manager) error {
	schemas, err := manager.ListSchemas()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to list schemas", err)
	}

	if len(schemas) == 0 {
		fmt.Println("No schemas found.")
		fmt.Println()
		fmt.Println("💡 Add your first schema:")
		fmt.Println("   pudl schema add aws.ec2-instance my-schema.cue")
		return nil
	}

	fmt.Println("Available Schemas:")
	fmt.Println()

	totalSchemas := 0
	for packageName, packageSchemas := range schemas {
		fmt.Printf("📦 Package: %s (%d schemas)\n", packageName, len(packageSchemas))

		for _, schemaInfo := range packageSchemas {
			totalSchemas++
			if schemaVerbose {
				fmt.Printf("   ├─ %s\n", schemaInfo.Name)
				fmt.Printf("   │  File: %s\n", schemaInfo.FilePath)
				fmt.Printf("   │  Size: %s\n", formatBytes(schemaInfo.Size))
				if schemaInfo.Definition != "" {
					fmt.Printf("   │  Definition: %s\n", schemaInfo.Definition)
				}
				fmt.Printf("   │\n")
			} else {
				definitionInfo := ""
				if schemaInfo.Definition != "" {
					definitionInfo = fmt.Sprintf(" (%s)", schemaInfo.Definition)
				}
				fmt.Printf("   ├─ %s%s\n", schemaInfo.Name, definitionInfo)
			}
		}
		fmt.Println()
	}

	fmt.Printf("Total: %d schemas in %d packages\n", totalSchemas, len(schemas))
	return nil
}

// listSchemasInPackage lists schemas in a specific package
func listSchemasInPackage(manager *schema.Manager, packageName string) error {
	schemas, err := manager.GetSchemasInPackage(packageName)
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem,
			fmt.Sprintf("Failed to list schemas in package '%s'", packageName), err)
	}

	if len(schemas) == 0 {
		fmt.Printf("No schemas found in package '%s'.\n", packageName)
		return nil
	}

	fmt.Printf("Schemas in package '%s':\n", packageName)
	fmt.Println()

	for _, schemaInfo := range schemas {
		if schemaVerbose {
			fmt.Printf("📄 %s\n", schemaInfo.Name)
			fmt.Printf("   File: %s\n", schemaInfo.FilePath)
			fmt.Printf("   Size: %s\n", formatBytes(schemaInfo.Size))
			if schemaInfo.Definition != "" {
				fmt.Printf("   Definition: %s\n", schemaInfo.Definition)
			}
			fmt.Println()
		} else {
			definitionInfo := ""
			if schemaInfo.Definition != "" {
				definitionInfo = fmt.Sprintf(" (%s)", schemaInfo.Definition)
			}
			fmt.Printf("  %s%s\n", schemaInfo.Name, definitionInfo)
		}
	}

	if !schemaVerbose {
		fmt.Printf("\nTotal: %d schemas\n", len(schemas))
	}
	return nil
}


