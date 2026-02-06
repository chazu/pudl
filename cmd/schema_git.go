package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
	"pudl/internal/git"
)

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

func init() {
	schemaCmd.AddCommand(schemaStatusCmd)
	schemaCmd.AddCommand(schemaCommitCmd)
	schemaCmd.AddCommand(schemaLogCmd)

	// Status command flags
	schemaStatusCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show detailed file status")

	// Commit command flags
	schemaCommitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "Commit message (required)")
	schemaCommitCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show detailed commit information")

	// Log command flags
	schemaLogCmd.Flags().IntVar(&logLimit, "limit", 10, "Number of commits to show")
	schemaLogCmd.Flags().BoolVarP(&schemaVerbose, "verbose", "v", false, "Show detailed commit information")
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
