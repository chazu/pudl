package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
)

// moduleCmd represents the module command
var moduleCmd = &cobra.Command{
	Use:   "module",
	Short: "Manage CUE module dependencies",
	Long: `Manage CUE module dependencies for PUDL schemas.

This command provides utilities for managing third-party CUE modules
that provide schemas for common data formats like Kubernetes, AWS, etc.

Examples:
    pudl module tidy     # Fetch and update module dependencies
    pudl module list     # List current module dependencies
    pudl module info     # Show module information`,
}

// moduleTidyCmd represents the module tidy command
var moduleTidyCmd = &cobra.Command{
	Use:   "tidy",
	Short: "Fetch and update module dependencies",
	Long: `Fetch and update CUE module dependencies.

This command runs 'cue mod tidy' in the schema directory to:
- Download missing dependencies
- Update the module.cue file with resolved versions
- Clean up unused dependencies

This is equivalent to running 'cue mod tidy' manually in the schema directory.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runModuleTidyCommand(); err != nil {
			errorHandler := errors.NewCLIErrorHandler(true)
			errorHandler.HandleError(err)
		}
	},
}

// moduleListCmd represents the module list command
var moduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List current module dependencies",
	Long: `List the current CUE module dependencies defined in cue.mod/module.cue.

This command shows:
- Module path and version
- Dependency versions
- Module description`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runModuleListCommand(); err != nil {
			errorHandler := errors.NewCLIErrorHandler(true)
			errorHandler.HandleError(err)
		}
	},
}

// moduleInfoCmd represents the module info command
var moduleInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show module information",
	Long: `Show information about the current CUE module.

This command displays:
- Module path and version
- CUE language version
- Source information
- Dependencies count`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runModuleInfoCommand(); err != nil {
			errorHandler := errors.NewCLIErrorHandler(true)
			errorHandler.HandleError(err)
		}
	},
}

// moduleAddCmd represents the module add command
var moduleAddCmd = &cobra.Command{
	Use:   "add <module@version>",
	Short: "Add a third-party module dependency",
	Long: `Add a third-party CUE module dependency to the current module.

This command modifies the cue.mod/module.cue file to include the specified
dependency and then runs 'cue mod tidy' to fetch it.

Examples:
    pudl module add cue.dev/x/k8s.io@v0
    pudl module add github.com/example/schemas@v1`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runModuleAddCommand(args[0]); err != nil {
			errorHandler := errors.NewCLIErrorHandler(true)
			errorHandler.HandleError(err)
		}
	},
}

func runModuleTidyCommand() error {
	// Load configuration to get schema path
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Check if CUE is available
	if _, err := exec.LookPath("cue"); err != nil {
		return errors.NewSystemError("CUE command not found", fmt.Errorf("install CUE from https://cuelang.org/docs/install/"))
	}

	// Check if module.cue exists
	modulePath := filepath.Join(cfg.SchemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError("cue.mod/module.cue not found - run 'pudl init' first")
	}

	fmt.Println("Fetching CUE module dependencies...")

	// Run cue mod tidy
	cmd := exec.Command("cue", "mod", "tidy")
	cmd.Dir = cfg.SchemaPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.NewSystemError("Failed to run 'cue mod tidy'", err)
	}

	fmt.Println("✅ Module dependencies updated successfully")
	return nil
}

func runModuleListCommand() error {
	// Load configuration to get schema path
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Check if module.cue exists
	modulePath := filepath.Join(cfg.SchemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError("cue.mod/module.cue not found - run 'pudl init' first")
	}

	// Read and display module.cue content
	content, err := os.ReadFile(modulePath)
	if err != nil {
		return errors.NewFileNotFoundError("Failed to read module.cue")
	}

	fmt.Println("CUE Module Configuration:")
	fmt.Println("========================")
	fmt.Printf("%s\n", content)

	return nil
}

func runModuleInfoCommand() error {
	// Load configuration to get schema path
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Check if module.cue exists
	modulePath := filepath.Join(cfg.SchemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError("cue.mod/module.cue not found - run 'pudl init' first")
	}

	fmt.Printf("Module Information:\n")
	fmt.Printf("==================\n")
	fmt.Printf("Schema Directory: %s\n", cfg.SchemaPath)
	fmt.Printf("Module File: %s\n", modulePath)

	// Show additional module information if CUE is available
	if _, err := exec.LookPath("cue"); err == nil {
		fmt.Println("\nModule Dependencies:")
		fmt.Println("===================")

		// Try to show module dependencies using cue mod edit
		cmd := exec.Command("cue", "mod", "edit", "--json")
		cmd.Dir = cfg.SchemaPath
		if output, err := cmd.Output(); err == nil {
			fmt.Printf("%s\n", output)
		} else {
			fmt.Println("No dependencies or unable to read module information")
		}
	} else {
		fmt.Println("\n⚠️  CUE command not available - install from https://cuelang.org/docs/install/")
	}

	return nil
}

func runModuleAddCommand(moduleSpec string) error {
	// Load configuration to get schema path
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Check if CUE is available
	if _, err := exec.LookPath("cue"); err != nil {
		return errors.NewSystemError("CUE command not found", fmt.Errorf("install CUE from https://cuelang.org/docs/install/"))
	}

	// Check if module.cue exists
	modulePath := filepath.Join(cfg.SchemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError("cue.mod/module.cue not found - run 'pudl init' first")
	}

	fmt.Printf("Adding module dependency: %s\n", moduleSpec)

	// Use cue mod get to add the dependency
	cmd := exec.Command("cue", "mod", "get", moduleSpec)
	cmd.Dir = cfg.SchemaPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.NewSystemError("Failed to add module dependency", err)
	}

	fmt.Printf("✅ Module dependency %s added successfully\n", moduleSpec)
	fmt.Println("You can now import packages from this module in your CUE files.")

	return nil
}

func init() {
	// Add module command to root
	rootCmd.AddCommand(moduleCmd)

	// Add subcommands
	moduleCmd.AddCommand(moduleTidyCmd)
	moduleCmd.AddCommand(moduleListCmd)
	moduleCmd.AddCommand(moduleInfoCmd)
	moduleCmd.AddCommand(moduleAddCmd)
}
