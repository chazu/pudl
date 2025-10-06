package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
)

// gitCmd represents the git command
var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Git operations for PUDL schema repository",
	Long: `Git operations and utilities for the PUDL schema repository.

The PUDL schema repository is located at ~/.pudl/schema/ and contains all
CUE schema definitions used for data validation and organization.

Available subcommands:
- cd: Get the path to the schema repository or generate a cd command

Examples:
    pudl git cd                    # Show the schema repository path
    eval $(pudl git cd --shell)    # Change to schema repository (bash/zsh)
    pudl git cd --shell            # Show shell command to change directory`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: show help
		cmd.Help()
	},
}

var (
	gitCdShell bool
)

// gitCdCmd represents the git cd command
var gitCdCmd = &cobra.Command{
	Use:   "cd",
	Short: "Navigate to the PUDL schema repository",
	Long: `Navigate to the PUDL schema repository directory.

Since a subprocess cannot change the parent shell's directory, this command
provides different output modes:

Default mode: Shows the path to the schema repository
Shell mode: Outputs a shell command that can be executed or sourced

The schema repository is located at ~/.pudl/schema/ and contains all CUE
schema definitions organized by packages (aws, k8s, custom, etc.).

Examples:
    pudl git cd                    # Show repository path
    eval $(pudl git cd --shell)    # Change directory (bash/zsh)
    pudl git cd --shell            # Show cd command
    
For fish shell:
    eval (pudl git cd --shell)     # Change directory (fish)`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the git cd command and handle any errors
		if err := runGitCdCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runGitCdCommand contains the actual git cd logic with structured error handling
func runGitCdCommand(cmd *cobra.Command, args []string) error {
	// Load configuration to get schema path
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Check if schema directory exists
	if _, err := os.Stat(cfg.SchemaPath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError(cfg.SchemaPath)
	}

	if gitCdShell {
		// Output shell command for evaluation
		fmt.Printf("cd %s", cfg.SchemaPath)
	} else {
		// Default mode: show path and instructions
		fmt.Printf("PUDL Schema Repository: %s\n", cfg.SchemaPath)
		fmt.Println()
		fmt.Println("💡 To navigate to this directory:")
		
		// Detect shell and provide appropriate instructions
		shell := os.Getenv("SHELL")
		if shell == "" {
			// Fallback based on OS
			if runtime.GOOS == "windows" {
				fmt.Printf("   cd \"%s\"\n", cfg.SchemaPath)
			} else {
				fmt.Printf("   cd %s\n", cfg.SchemaPath)
			}
		} else {
			// Provide shell-specific instructions
			fmt.Printf("   cd %s\n", cfg.SchemaPath)
			fmt.Println()
			fmt.Println("🚀 Or use the shell integration:")
			if shell == "/bin/fish" || shell == "/usr/bin/fish" {
				fmt.Printf("   eval (pudl git cd --shell)\n")
			} else {
				fmt.Printf("   eval $(pudl git cd --shell)\n")
			}
		}
		
		fmt.Println()
		fmt.Println("📁 This directory contains:")
		fmt.Println("   - CUE schema files organized by packages")
		fmt.Println("   - Git repository for version control")
		fmt.Println("   - Package directories: aws/, k8s/, custom/, unknown/")
	}

	return nil
}

func init() {
	// Add git command to root
	rootCmd.AddCommand(gitCmd)

	// Add cd subcommand to git
	gitCmd.AddCommand(gitCdCmd)

	// Add flags for git cd command
	gitCdCmd.Flags().BoolVar(&gitCdShell, "shell", false, "Output shell command for evaluation")
}
