package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/errors"
	"pudl/internal/git"
	"pudl/internal/idgen"
	"pudl/internal/inference"
	"pudl/internal/review"
	"pudl/internal/schema"
	"pudl/internal/schemagen"
)

var (
	schemaVerbose bool
	schemaPackage string
	commitMessage string
	logLimit      int

	// Review command flags
	reviewSchema      string
	reviewOrigin      string
	reviewFormat      string
	reviewMaxItems    int
	reviewOnlyUnknown bool
	reviewSessionID   string
	reviewListSessions bool

	// New command flags
	schemaNewFrom       string
	schemaNewPath       string
	schemaNewCollection bool
	schemaNewInfer      []string

	// Reinfer command flags
	reinferAll         bool
	reinferEntry       string
	reinferSchema      string
	reinferOrigin      string
	reinferDryRun      bool
	reinferForce       bool
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

	// Show definitions found in the added file
	if len(result.Definitions) > 0 {
		fmt.Printf("   Package: %s\n", packageName)
		fmt.Printf("   Definitions: %s\n", strings.Join(result.Definitions, ", "))
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

// schemaReviewCmd represents the schema review command
var schemaReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Interactively review and improve schema assignments",
	Long: `Start an interactive review session to improve schema assignments for your data.

This command provides a git rebase-like workflow for reviewing data items and their
schema assignments. You can:

- Accept current schema assignments
- Reassign items to different existing schemas
- Create new schemas from data patterns
- Skip items for later review

The review session is persistent and resumable - you can quit at any time and
continue later. All changes are tracked and can be committed to version control.

Examples:
    # Review all items with unknown schemas
    pudl schema review --only-unknown

    # Review specific schema assignments
    pudl schema review --schema unknown.#CatchAll

    # Review items from specific origin
    pudl schema review --origin aws-ec2

    # Resume a previous session
    pudl schema review --session review-1634567890

    # List available sessions
    pudl schema review --list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaReview()
	},
}

// schemaNewCmd represents the schema new command
var schemaNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Generate a new schema from imported data",
	Long: `Generate a new CUE schema by analyzing data from a previously imported entry.

This command creates a new schema file based on the structure of imported data,
inferring field types, identifying likely identity fields, and generating
appropriate CUE type definitions.

The --from flag specifies the proquint ID of the imported data to analyze.
The --path flag specifies where to create the schema (package path and definition name).

If the path contains a # character, everything after it is used as the definition name.
Otherwise, the last path component is capitalized and used as the definition name.

When --from points to a collection entry:
- Without --collection: analyzes individual items to create an item schema
- With --collection: creates a schema for the collection structure itself

The --infer flag allows specifying type hints for specific fields, such as
marking a field as an enum type.

Examples:
    pudl schema new --from hugib-dubuf --path aws/ec2:#Instance
    pudl schema new --from govim-nupab --path aws/ec2:#Instance  # from collection
    pudl schema new --from govim-nupab --path aws/ec2:#InstanceCollection --collection
    pudl schema new --from hugib-dubuf --path aws/ec2:#Instance --infer State=enum`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaNewCommand()
	},
}

// schemaEditCmd represents the schema edit command
var schemaEditCmd = &cobra.Command{
	Use:   "edit <path>",
	Short: "Open a schema file in your editor",
	Long: `Open a schema file in your configured editor.

The path can optionally include a definition name to position the cursor at that
definition (supported for vim/nvim editors).

Examples:
    pudl schema edit aws/ec2:#Instance    # Opens the file and positions at #Instance if possible
    pudl schema edit aws/ec2              # Opens the aws/ec2.cue file`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaEditCommand(args[0])
	},
}

// schemaReinferCmd represents the schema reinfer command
var schemaReinferCmd = &cobra.Command{
	Use:   "reinfer",
	Short: "Re-run schema inference on existing catalog entries",
	Long: `Re-run schema inference on data that has already been imported.

This command is useful when:
- New schemas have been added and you want existing data to match against them
- Schema cascade priorities have been modified
- You want to batch-update schema assignments without interactive review

The command will re-analyze each matching entry and update its schema assignment
if a better match is found.

Examples:
    # Re-infer all entries currently assigned to unknown schema
    pudl schema reinfer --schema unknown.#CatchAll

    # Re-infer a specific entry
    pudl schema reinfer --entry babod-fakak

    # Preview what would change for all entries
    pudl schema reinfer --all --dry-run

    # Re-infer all entries from a specific origin
    pudl schema reinfer --origin aws-ec2

    # Force re-inference without confirmation
    pudl schema reinfer --all --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaReinferCommand()
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)

	// Add subcommands
	schemaCmd.AddCommand(schemaListCmd)
	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaStatusCmd)
	schemaCmd.AddCommand(schemaCommitCmd)
	schemaCmd.AddCommand(schemaLogCmd)
	schemaCmd.AddCommand(schemaReviewCmd)
	schemaCmd.AddCommand(schemaNewCmd)
	schemaCmd.AddCommand(schemaEditCmd)
	schemaCmd.AddCommand(schemaReinferCmd)

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

	// Review command flags
	schemaReviewCmd.Flags().StringVar(&reviewSchema, "schema", "", "Review only items with this schema")
	schemaReviewCmd.Flags().StringVar(&reviewOrigin, "origin", "", "Review only items from this origin")
	schemaReviewCmd.Flags().StringVar(&reviewFormat, "format", "", "Review only items with this format")
	schemaReviewCmd.Flags().IntVar(&reviewMaxItems, "max-items", 50, "Maximum number of items to review")
	schemaReviewCmd.Flags().BoolVar(&reviewOnlyUnknown, "only-unknown", false, "Review only items with unknown schemas")
	schemaReviewCmd.Flags().StringVar(&reviewSessionID, "session", "", "Resume a specific review session")
	schemaReviewCmd.Flags().BoolVar(&reviewListSessions, "list", false, "List available review sessions")

	// New command flags
	schemaNewCmd.Flags().StringVar(&schemaNewFrom, "from", "", "Proquint ID of the imported data to generate schema from (required)")
	schemaNewCmd.Flags().StringVar(&schemaNewPath, "path", "", "Schema path in format 'package/path:#Definition' (required)")
	schemaNewCmd.Flags().BoolVar(&schemaNewCollection, "collection", false, "Create a collection schema instead of item schema")
	schemaNewCmd.Flags().StringArrayVar(&schemaNewInfer, "infer", []string{}, "Field inference hints (e.g., State=enum)")
	schemaNewCmd.MarkFlagRequired("from")
	schemaNewCmd.MarkFlagRequired("path")

	// Reinfer command flags
	schemaReinferCmd.Flags().BoolVar(&reinferAll, "all", false, "Re-infer schemas for all catalog entries")
	schemaReinferCmd.Flags().StringVar(&reinferEntry, "entry", "", "Re-infer schema for a specific entry by proquint ID")
	schemaReinferCmd.Flags().StringVar(&reinferSchema, "schema", "", "Re-infer only entries currently assigned to this schema")
	schemaReinferCmd.Flags().StringVar(&reinferOrigin, "origin", "", "Re-infer only entries from a specific origin")
	schemaReinferCmd.Flags().BoolVar(&reinferDryRun, "dry-run", false, "Show what would change without applying updates")
	schemaReinferCmd.Flags().BoolVar(&reinferForce, "force", false, "Apply changes without confirmation prompt")
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
				fmt.Printf("   │  Full name: %s\n", schemaInfo.FullName)
				fmt.Printf("   │  File: %s\n", schemaInfo.FilePath)
				fmt.Printf("   │  Size: %s\n", formatBytes(schemaInfo.Size))
				fmt.Printf("   │\n")
			} else {
				fmt.Printf("   ├─ %s\n", schemaInfo.Name)
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
			fmt.Printf("   Full name: %s\n", schemaInfo.FullName)
			fmt.Printf("   File: %s\n", schemaInfo.FilePath)
			fmt.Printf("   Size: %s\n", formatBytes(schemaInfo.Size))
			fmt.Println()
		} else {
			fmt.Printf("  %s\n", schemaInfo.Name)
		}
	}

	if !schemaVerbose {
		fmt.Printf("\nTotal: %d schemas\n", len(schemas))
	}
	return nil
}

// runSchemaReview runs the interactive schema review workflow
func runSchemaReview() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to load configuration", err)
	}

	fmt.Printf("🔍 Schema Review Workflow\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")

	if reviewListSessions {
		return listReviewSessions(cfg)
	}

	// Resume existing session if specified
	if reviewSessionID != "" {
		return resumeReviewSession(cfg, reviewSessionID)
	}

	// Start new review session
	return startNewReviewSession(cfg)
}

// startNewReviewSession starts a new schema review session
func startNewReviewSession(cfg *config.Config) error {
	fmt.Printf("🚀 Starting new schema review session...\n\n")

	// Initialize database connection
	catalogDB, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	// Create session manager
	sessionMgr := review.NewSessionManager(cfg.DataPath)

	// Create review item fetcher
	fetcher := review.NewReviewItemFetcher(catalogDB, cfg.DataPath)

	// Build session filter from command line flags
	filter := review.SessionFilter{
		Schema:      reviewSchema,
		Origin:      reviewOrigin,
		Format:      reviewFormat,
		MaxItems:    reviewMaxItems,
		OnlyUnknown: reviewOnlyUnknown,
	}

	// Show preview of what will be reviewed
	stats, err := fetcher.GetReviewStats(filter)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get review statistics", err)
	}

	fmt.Printf("📊 Review Preview:\n")
	fmt.Printf("  Total items matching criteria: %d\n", stats.TotalItems)
	if stats.UnknownItems > 0 {
		fmt.Printf("  Items with unknown schemas: %d\n", stats.UnknownItems)
	}
	fmt.Printf("  Items to review (max): %d\n", min(stats.TotalItems, reviewMaxItems))

	if len(stats.SchemaBreakdown) > 0 {
		fmt.Printf("\n📋 Schema Breakdown:\n")
		for schema, count := range stats.SchemaBreakdown {
			fmt.Printf("  %s: %d items\n", schema, count)
		}
	}

	if stats.TotalItems == 0 {
		fmt.Printf("\n❌ No items found matching the specified criteria.\n")
		fmt.Printf("💡 Try adjusting your filters or import more data.\n")
		return nil
	}

	// Confirm before starting
	fmt.Printf("\nProceed with review? [y/N]: ")
	var response string
	fmt.Scanln(&response)
	if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
		fmt.Printf("Review cancelled.\n")
		return nil
	}

	// Fetch items for review
	fmt.Printf("\n📥 Fetching items for review...\n")
	items, err := fetcher.FetchItemsForReview(filter)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to fetch items for review", err)
	}

	// Create review session
	session, err := sessionMgr.CreateSession(items, filter)
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to create review session", err)
	}

	fmt.Printf("✅ Created review session: %s\n", session.SessionID)
	fmt.Printf("📝 %d items ready for review\n\n", len(items))

	// Launch interactive reviewer
	return launchInteractiveReviewer(cfg, session, sessionMgr, catalogDB)
}

// resumeReviewSession resumes an existing review session
func resumeReviewSession(cfg *config.Config, sessionID string) error {
	fmt.Printf("🔄 Resuming review session: %s\n\n", sessionID)

	// Initialize database connection
	catalogDB, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	// Create session manager
	sessionMgr := review.NewSessionManager(cfg.DataPath)

	// Load existing session
	session, err := sessionMgr.LoadSession(sessionID)
	if err != nil {
		return err // Already wrapped by session manager
	}

	fmt.Printf("📊 Session Status:\n")
	fmt.Printf("  Progress: %d/%d items (%.1f%%)\n",
		session.CurrentIndex, len(session.Items), session.GetProgress())
	fmt.Printf("  Changes made: %d\n", len(session.Changes))
	fmt.Printf("  State: %s\n\n", session.State)

	// Launch interactive reviewer
	return launchInteractiveReviewer(cfg, session, sessionMgr, catalogDB)
}

// launchInteractiveReviewer launches the interactive review interface
func launchInteractiveReviewer(cfg *config.Config, session *review.ReviewSession, sessionMgr *review.SessionManager, catalogDB *database.CatalogDB) error {
	// Create schema manager
	schemaMgr := schema.NewManager(cfg.SchemaPath)

	// Create validator
	validator := schema.NewValidator()

	// Create catalog updater
	catalogUpdater := review.NewCatalogUpdater(catalogDB)

	// Create interactive reviewer
	reviewer, err := review.NewInteractiveReviewer(session, sessionMgr, schemaMgr, validator, catalogUpdater, cfg.SchemaPath)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to create interactive reviewer", err)
	}

	// Run the review workflow
	return reviewer.RunReview()
}

// listReviewSessions lists available review sessions
func listReviewSessions(cfg *config.Config) error {
	fmt.Printf("📋 Available Review Sessions\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")

	// Create session manager
	sessionMgr := review.NewSessionManager(cfg.DataPath)

	// List sessions
	sessions, err := sessionMgr.ListSessions()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to list review sessions", err)
	}

	if len(sessions) == 0 {
		fmt.Printf("No review sessions found.\n")
		fmt.Printf("💡 Start a new session with: pudl schema review --only-unknown\n")
		return nil
	}

	fmt.Printf("Found %d review sessions:\n\n", len(sessions))

	for i, session := range sessions {
		status := "🔄"
		if session.State == review.SessionCompleted {
			status = "✅"
		} else if session.State == review.SessionAborted {
			status = "❌"
		}

		fmt.Printf("%d. %s %s\n", i+1, status, session.SessionID)
		fmt.Printf("   Progress: %d/%d items (%.1f%%)\n",
			session.CurrentIndex, len(session.Items), session.GetProgress())
		fmt.Printf("   Changes: %d | State: %s\n", len(session.Changes), session.State)
		fmt.Printf("   Started: %s\n", session.StartTime.Format("2006-01-02 15:04:05"))

		if session.EndTime != nil {
			fmt.Printf("   Ended: %s\n", session.EndTime.Format("2006-01-02 15:04:05"))
		}

		fmt.Printf("   Filter: ")
		if session.Filter.OnlyUnknown {
			fmt.Printf("unknown schemas only")
		} else if session.Filter.Schema != "" {
			fmt.Printf("schema=%s", session.Filter.Schema)
		} else {
			fmt.Printf("all items")
		}
		fmt.Printf("\n\n")
	}

	fmt.Printf("💡 Resume a session with: pudl schema review --session <session-id>\n")
	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// runSchemaNewCommand runs the schema new command
func runSchemaNewCommand() error {
	// Parse the --path flag to extract package path and definition name
	packagePath, definitionName := parseSchemaPath(schemaNewPath)

	// Parse --infer flags into map
	inferHints := parseInferHints(schemaNewInfer)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Initialize database connection
	catalogDB, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	// Look up the entry by proquint ID
	entry, err := catalogDB.GetEntryByProquint(schemaNewFrom)
	if err != nil {
		return err
	}

	// Create the generator
	generator := schemagen.NewGenerator(cfg.SchemaPath)

	// Check if this is a collection entry with --collection flag (smart collection generation)
	if entry.CollectionType != nil && *entry.CollectionType == "collection" && schemaNewCollection {
		return runSmartCollectionGeneration(catalogDB, generator, entry, packagePath, definitionName, inferHints, cfg)
	}

	// Standard schema generation (item schema or legacy collection)
	var data interface{}
	if entry.CollectionType != nil && *entry.CollectionType == "collection" && !schemaNewCollection {
		// It's a collection and --collection is NOT set: get all items and merge their data
		items, err := catalogDB.GetCollectionItems(entry.ID)
		if err != nil {
			return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get collection items", err)
		}

		if len(items) == 0 {
			return errors.NewInputError("Collection has no items",
				"Use --collection flag to create a schema for the collection itself")
		}

		// Load data from each item and create an array for the generator
		var itemsData []interface{}
		for _, item := range items {
			itemData, err := loadJSONFile(item.StoredPath)
			if err != nil {
				return errors.WrapError(errors.ErrCodeFileSystem,
					fmt.Sprintf("Failed to load item data from %s", item.StoredPath), err)
			}
			itemsData = append(itemsData, itemData)
		}
		data = itemsData
	} else {
		// Either it's a single item, or it's a collection with --collection set
		data, err = loadJSONFile(entry.StoredPath)
		if err != nil {
			return errors.WrapError(errors.ErrCodeFileSystem,
				fmt.Sprintf("Failed to load data from %s", entry.StoredPath), err)
		}
	}

	// Build generation options
	opts := schemagen.GenerateOptions{
		FromID:         entry.ID,
		PackagePath:    packagePath,
		DefinitionName: definitionName,
		IsCollection:   schemaNewCollection,
		InferHints:     inferHints,
	}

	// Generate the schema
	result, err := generator.Generate(data, opts)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to generate schema", err)
	}

	// Write the schema file
	if err := generator.WriteSchema(result, result.Content); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to write schema file", err)
	}

	// Print results
	fmt.Println("✅ Schema generated successfully!")
	fmt.Println()
	fmt.Printf("📄 File created: %s\n", result.FilePath)
	fmt.Printf("📦 Package: %s\n", result.PackageName)
	fmt.Printf("📋 Definition: #%s\n", result.DefinitionName)
	fmt.Printf("🔢 Fields: %d\n", result.FieldCount)

	if len(result.InferredIdentityFields) > 0 {
		fmt.Printf("🔑 Inferred identity fields: %s\n", strings.Join(result.InferredIdentityFields, ", "))
	}

	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Printf("   - Edit the schema: pudl schema edit %s:#%s\n", packagePath, result.DefinitionName)
	fmt.Printf("   - Commit changes: pudl schema commit -m \"Add %s schema\"\n", result.DefinitionName)

	return nil
}

// runSmartCollectionGeneration handles smart collection schema generation.
// It infers schemas for each item, reuses existing schemas where possible,
// generates new item schemas for unmatched items, and creates a list-type collection schema.
func runSmartCollectionGeneration(catalogDB *database.CatalogDB, generator *schemagen.Generator, entry *database.CatalogEntry, packagePath, definitionName string, inferHints map[string]string, cfg *config.Config) error {
	// Get collection items
	items, err := catalogDB.GetCollectionItems(entry.ID)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to get collection items", err)
	}

	if len(items) == 0 {
		return errors.NewInputError("Collection has no items", "Cannot generate collection schema from empty collection")
	}

	// Load data from each item
	var itemsData []interface{}
	for _, item := range items {
		itemData, err := loadJSONFile(item.StoredPath)
		if err != nil {
			return errors.WrapError(errors.ErrCodeFileSystem,
				fmt.Sprintf("Failed to load item data from %s", item.StoredPath), err)
		}
		itemsData = append(itemsData, itemData)
	}

	// Initialize the schema inferrer
	inferrer, err := inference.NewSchemaInferrer(cfg.SchemaPath)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to initialize schema inferrer", err)
	}

	// Build collection generation options
	opts := schemagen.CollectionGenerateOptions{
		PackagePath:    packagePath,
		CollectionName: definitionName,
		InferHints:     inferHints,
	}

	// Generate the smart collection
	result, err := generator.GenerateSmartCollection(itemsData, opts, inferrer)
	if err != nil {
		return errors.WrapError(errors.ErrCodeValidationFailed, "Failed to generate collection schema", err)
	}

	// Write any new item schemas first
	for _, itemSchema := range result.NewItemSchemas {
		if err := generator.WriteSchema(itemSchema, itemSchema.Content); err != nil {
			return errors.WrapError(errors.ErrCodeFileSystem,
				fmt.Sprintf("Failed to write item schema file: %s", itemSchema.FilePath), err)
		}
		fmt.Printf("📄 Created item schema: %s\n", itemSchema.FilePath)
	}

	// Write the collection schema
	if err := generator.WriteSchema(result.CollectionSchema, result.CollectionSchema.Content); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem,
			fmt.Sprintf("Failed to write collection schema file: %s", result.CollectionSchema.FilePath), err)
	}

	// Print results
	fmt.Println()
	fmt.Println("✅ Collection schema generated successfully!")
	fmt.Println()
	fmt.Printf("📄 Collection schema: %s\n", result.CollectionSchema.FilePath)
	fmt.Printf("📦 Package: %s\n", result.CollectionSchema.PackageName)
	fmt.Printf("📋 Definition: #%s\n", result.CollectionSchema.DefinitionName)

	if len(result.ExistingSchemaRefs) > 0 {
		fmt.Println()
		fmt.Println("🔗 Reused existing schemas:")
		for _, ref := range result.ExistingSchemaRefs {
			fmt.Printf("   - %s\n", ref)
		}
	}

	if len(result.NewItemSchemas) > 0 {
		fmt.Println()
		fmt.Println("✨ Generated new item schemas:")
		for _, schema := range result.NewItemSchemas {
			fmt.Printf("   - #%s (%d fields)\n", schema.DefinitionName, schema.FieldCount)
		}
	}

	fmt.Println()
	fmt.Println("💡 Next steps:")
	fmt.Printf("   - Edit the collection schema: pudl schema edit %s:#%s\n", packagePath, definitionName)
	if len(result.NewItemSchemas) > 0 {
		fmt.Printf("   - Edit item schemas as needed\n")
	}
	fmt.Printf("   - Commit changes: pudl schema commit -m \"Add %s collection schema\"\n", definitionName)

	return nil
}

// parseSchemaPath parses the --path flag into package path and definition name
// e.g., "aws/ec2:#Instance" → ("aws/ec2", "Instance")
// e.g., "aws/ec2" → ("aws/ec2", "Ec2")
func parseSchemaPath(path string) (packagePath, definitionName string) {
	// Check if path contains # for explicit definition name
	if idx := strings.Index(path, ":#"); idx != -1 {
		packagePath = path[:idx]
		definitionName = path[idx+2:] // Skip :#
		return
	}

	// No explicit definition, capitalize the last path component
	packagePath = path
	parts := strings.Split(path, "/")
	lastPart := parts[len(parts)-1]
	definitionName = capitalizeFirst(lastPart)
	return
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// parseInferHints parses --infer flags into a map
// e.g., ["State=enum", "Type=enum"] → {"State": "enum", "Type": "enum"}
func parseInferHints(hints []string) map[string]string {
	result := make(map[string]string)
	for _, hint := range hints {
		parts := strings.SplitN(hint, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

// loadJSONFile loads and parses a JSON file
func loadJSONFile(path string) (interface{}, error) {
	fileData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var data interface{}
	if err := json.Unmarshal(fileData, &data); err != nil {
		return nil, err
	}

	return data, nil
}

// runSchemaEditCommand opens a schema file in the user's editor
func runSchemaEditCommand(pathArg string) error {
	// Parse the path argument to extract package path and optional definition name
	packagePath, definitionName := parseSchemaEditPath(pathArg)

	// Load configuration to get schema path
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Resolve the file path
	// Schema files can be at:
	// 1. ~/.pudl/schema/<package_path>/<definition>.cue (created by pudl schema new)
	// 2. ~/.pudl/schema/<package_path>.cue (simple single-file schema)
	var filePath string
	var found bool

	// If definition name is provided, first try the definition-based path
	if definitionName != "" {
		// Try: aws/ec2 + Instance -> aws/ec2/instance.cue
		defPath := filepath.Join(cfg.SchemaPath, packagePath, strings.ToLower(definitionName)+".cue")
		if _, err := os.Stat(defPath); err == nil {
			filePath = defPath
			found = true
		}
	}

	// If not found, try the simple package path
	if !found {
		simplePath := filepath.Join(cfg.SchemaPath, packagePath+".cue")
		if _, err := os.Stat(simplePath); err == nil {
			filePath = simplePath
			found = true
		}
	}

	// If still not found, return error with helpful message
	if !found {
		if definitionName != "" {
			return errors.NewFileNotFoundError(
				fmt.Sprintf("%s/%s.cue or %s.cue",
					filepath.Join(cfg.SchemaPath, packagePath),
					strings.ToLower(definitionName),
					filepath.Join(cfg.SchemaPath, packagePath)))
		}
		return errors.NewFileNotFoundError(filepath.Join(cfg.SchemaPath, packagePath+".cue"))
	}

	// Get the editor from $EDITOR environment variable
	editor := os.Getenv("EDITOR")
	if editor == "" {
		// Try vi first, then nano as fallback
		editor = "vi"
	}

	// Build the command arguments
	var args []string

	// If a definition name was specified and we're using vim/nvim, try to position the cursor
	if definitionName != "" && isVimEditor(editor) {
		// Use vim's +/pattern command to search for the definition
		args = append(args, fmt.Sprintf("+/^#%s:", definitionName))
	}

	args = append(args, filePath)

	// Execute the editor
	cmd := exec.Command(editor, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem,
			fmt.Sprintf("Failed to open editor '%s'", editor), err)
	}

	return nil
}

// parseSchemaEditPath parses the path argument for the edit command
// e.g., "aws/ec2:#Instance" → ("aws/ec2", "Instance")
// e.g., "aws/ec2" → ("aws/ec2", "")
func parseSchemaEditPath(path string) (packagePath, definitionName string) {
	// Check if path contains :# for explicit definition name
	if idx := strings.Index(path, ":#"); idx != -1 {
		packagePath = path[:idx]
		definitionName = path[idx+2:] // Skip :#
		return
	}

	// No explicit definition
	packagePath = path
	definitionName = ""
	return
}

// isVimEditor checks if the editor is vim or nvim
func isVimEditor(editor string) bool {
	// Get the base name of the editor (in case it's a full path)
	baseName := filepath.Base(editor)
	return baseName == "vim" || baseName == "nvim" || baseName == "vi"
}

// runSchemaReinferCommand runs the schema reinfer command
func runSchemaReinferCommand() error {
	// Validate that at least one filter is specified
	if !reinferAll && reinferEntry == "" && reinferSchema == "" && reinferOrigin == "" {
		return errors.NewInputError(
			"At least one filter must be specified",
			"Use --all, --entry, --schema, or --origin to select entries to re-infer")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to load configuration", err)
	}

	// Derive config dir from data path
	configDir := filepath.Dir(cfg.DataPath)

	// Initialize catalog database
	catalogDB, err := database.NewCatalogDB(configDir)
	if err != nil {
		return errors.NewSystemError("Failed to initialize catalog database", err)
	}
	defer catalogDB.Close()

	// Create schema inferrer
	inferrer, err := inference.NewSchemaInferrer(cfg.SchemaPath)
	if err != nil {
		return errors.NewSystemError("Failed to initialize schema inferrer", err)
	}

	fmt.Printf("🔄 Schema Re-inference\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")

	// Handle single entry case
	if reinferEntry != "" {
		return reinferSingleEntry(catalogDB, inferrer, reinferEntry)
	}

	// Build filter for batch operation
	filter := database.FilterOptions{
		Schema: reinferSchema,
		Origin: reinferOrigin,
	}

	// Query matching entries
	queryResult, err := catalogDB.QueryEntries(filter, database.QueryOptions{
		Limit:   0, // No limit
		SortBy:  "import_timestamp",
		Reverse: true,
	})
	if err != nil {
		return errors.NewSystemError("Failed to query catalog entries", err)
	}

	if len(queryResult.Entries) == 0 {
		fmt.Println("No entries found matching the specified criteria.")
		return nil
	}

	fmt.Printf("Found %d entries to process\n\n", len(queryResult.Entries))

	// Analyze what would change
	changes := analyzeReinferChanges(queryResult.Entries, catalogDB, inferrer, cfg.DataPath)

	// Show summary
	printReinferSummary(changes)

	if reinferDryRun {
		fmt.Println("\n🔍 Dry run - no changes applied")
		return nil
	}

	if len(changes.updated) == 0 {
		fmt.Println("\n✅ No schema changes needed")
		return nil
	}

	// Confirm before applying
	if !reinferForce {
		fmt.Printf("\nApply %d schema changes? [y/N]: ", len(changes.updated))
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Apply changes
	return applyReinferChanges(catalogDB, changes)
}

// reinferSingleEntry re-infers schema for a single entry
func reinferSingleEntry(catalogDB *database.CatalogDB, inferrer *inference.SchemaInferrer, proquintID string) error {
	// Look up entry by proquint
	entry, err := catalogDB.GetEntryByProquint(proquintID)
	if err != nil {
		entry, err = catalogDB.GetEntry(proquintID)
		if err != nil {
			return errors.NewInputError(
				fmt.Sprintf("Entry not found: %s", proquintID),
				"Check the entry ID with 'pudl list'")
		}
	}

	// Load data from stored path
	data, err := loadReinferData(entry.StoredPath)
	if err != nil {
		return errors.NewSystemError(fmt.Sprintf("Failed to load data from %s", entry.StoredPath), err)
	}

	// Determine collection type for inference hints
	collectionType := ""
	if entry.CollectionType != nil {
		collectionType = *entry.CollectionType
	}

	// Run inference
	result, err := inferrer.Infer(data, inference.InferenceHints{
		Origin:         entry.Origin,
		Format:         entry.Format,
		CollectionType: collectionType,
	})
	if err != nil {
		return errors.NewSystemError("Schema inference failed", err)
	}

	proquint := idgen.HashToProquint(entry.ID)
	fmt.Printf("Entry: %s\n", proquint)
	fmt.Printf("Current schema: %s\n", entry.Schema)
	fmt.Printf("Inferred schema: %s (confidence: %.2f)\n", result.Schema, result.Confidence)

	if result.Schema == entry.Schema {
		fmt.Println("\n✅ Schema unchanged")
		return nil
	}

	if reinferDryRun {
		fmt.Println("\n🔍 Dry run - no changes applied")
		return nil
	}

	if !reinferForce {
		fmt.Printf("\nUpdate schema to %s? [y/N]: ", result.Schema)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Update the entry
	catalogUpdater := review.NewCatalogUpdater(catalogDB)
	if err := catalogUpdater.UpdateSingleEntry(entry.ID, result.Schema, result.Confidence); err != nil {
		return errors.NewSystemError("Failed to update catalog entry", err)
	}

	fmt.Printf("\n✅ Updated schema: %s → %s\n", entry.Schema, result.Schema)
	return nil
}

// reinferChanges holds the analysis of what would change
type reinferChanges struct {
	updated   []reinferChange
	unchanged []string
	errors    []string
}

type reinferChange struct {
	entryID     string
	proquint    string
	oldSchema   string
	newSchema   string
	confidence  float64
}

// analyzeReinferChanges analyzes what schema changes would occur
func analyzeReinferChanges(entries []database.CatalogEntry, catalogDB *database.CatalogDB, inferrer *inference.SchemaInferrer, dataPath string) *reinferChanges {
	changes := &reinferChanges{
		updated:   []reinferChange{},
		unchanged: []string{},
		errors:    []string{},
	}

	for _, entry := range entries {
		proquint := idgen.HashToProquint(entry.ID)

		// Load data
		data, err := loadReinferData(entry.StoredPath)
		if err != nil {
			changes.errors = append(changes.errors, fmt.Sprintf("%s: failed to load data", proquint))
			continue
		}

		// Determine collection type for inference hints
		collectionType := ""
		if entry.CollectionType != nil {
			collectionType = *entry.CollectionType
		}

		// Run inference
		result, err := inferrer.Infer(data, inference.InferenceHints{
			Origin:         entry.Origin,
			Format:         entry.Format,
			CollectionType: collectionType,
		})
		if err != nil {
			changes.errors = append(changes.errors, fmt.Sprintf("%s: inference failed", proquint))
			continue
		}

		if result.Schema != entry.Schema {
			changes.updated = append(changes.updated, reinferChange{
				entryID:    entry.ID,
				proquint:   proquint,
				oldSchema:  entry.Schema,
				newSchema:  result.Schema,
				confidence: result.Confidence,
			})
		} else {
			changes.unchanged = append(changes.unchanged, proquint)
		}
	}

	return changes
}

// printReinferSummary prints a summary of the reinfer analysis
func printReinferSummary(changes *reinferChanges) {
	fmt.Printf("📊 Analysis Summary:\n")
	fmt.Printf("   Would update: %d entries\n", len(changes.updated))
	fmt.Printf("   Unchanged:    %d entries\n", len(changes.unchanged))
	fmt.Printf("   Errors:       %d entries\n", len(changes.errors))

	if len(changes.updated) > 0 {
		fmt.Printf("\n📝 Schema changes:\n")
		for _, change := range changes.updated {
			fmt.Printf("   %s: %s → %s (%.2f)\n",
				change.proquint, change.oldSchema, change.newSchema, change.confidence)
		}
	}

	if len(changes.errors) > 0 {
		fmt.Printf("\n⚠️  Errors:\n")
		for _, errMsg := range changes.errors {
			fmt.Printf("   %s\n", errMsg)
		}
	}
}

// applyReinferChanges applies the analyzed schema changes
func applyReinferChanges(catalogDB *database.CatalogDB, changes *reinferChanges) error {
	catalogUpdater := review.NewCatalogUpdater(catalogDB)

	var successCount, failCount int
	for _, change := range changes.updated {
		if err := catalogUpdater.UpdateSingleEntry(change.entryID, change.newSchema, change.confidence); err != nil {
			fmt.Printf("   ❌ %s: failed to update\n", change.proquint)
			failCount++
		} else {
			fmt.Printf("   ✅ %s: %s → %s\n", change.proquint, change.oldSchema, change.newSchema)
			successCount++
		}
	}

	fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("✅ Updated: %d entries\n", successCount)
	if failCount > 0 {
		fmt.Printf("❌ Failed:  %d entries\n", failCount)
	}

	return nil
}

// loadReinferData loads data from a stored file path for re-inference
func loadReinferData(storedPath string) (interface{}, error) {
	data, err := os.ReadFile(storedPath)
	if err != nil {
		return nil, err
	}

	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return string(data), nil
	}

	return jsonData, nil
}
