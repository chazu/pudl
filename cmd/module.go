package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/proc"
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
	RunE: pudlRunE(func(cmd *cobra.Command, args []string) error {
		return runModuleTidyCommand(cmd.Context())
	}),
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
	RunE: pudlRunE(func(cmd *cobra.Command, args []string) error {
		return runModuleListCommand()
	}),
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
	RunE: pudlRunE(func(cmd *cobra.Command, args []string) error {
		return runModuleInfoCommand(cmd.Context())
	}),
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
	RunE: pudlRunE(func(cmd *cobra.Command, args []string) error {
		return runModuleAddCommand(cmd.Context(), args[0])
	}),
}

// requireCue reports a missing cue binary as the error every module command
// that shells out to it returns.
func requireCue() error {
	if !proc.Available("cue") {
		return errors.NewSystemError("CUE command not found", fmt.Errorf("install CUE from https://cuelang.org/docs/install/"))
	}
	return nil
}

// cueCommand runs `cue args...` in dir, bound to ctx so an interrupt stops it.
func cueCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}
	command := proc.Command(ctx, proc.DefaultGrace, "cue", args...)
	command.Dir = dir
	return command
}

func runModuleTidyCommand(ctx context.Context) error {
	// Load configuration to get schema path
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	if err := requireCue(); err != nil {
		return err
	}

	// Check if module.cue exists
	schemaPath := effectiveSchemaPath(cfg)
	modulePath := filepath.Join(schemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError("cue.mod/module.cue not found - run 'pudl init' first")
	}

	fmt.Fprintln(outw(), "Fetching CUE module dependencies...")

	// Run cue mod tidy
	cmd := cueCommand(ctx, schemaPath, "mod", "tidy")
	cmd.Stdout = outw()
	cmd.Stderr = errw()

	if err := cmd.Run(); err != nil {
		return errors.NewSystemError("Failed to run 'cue mod tidy'", err)
	}

	fmt.Fprintln(outw(), "✅ Module dependencies updated successfully")
	return nil
}

func runModuleListCommand() error {
	// Load configuration to get schema path
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Check if module.cue exists
	schemaPath := effectiveSchemaPath(cfg)
	modulePath := filepath.Join(schemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError("cue.mod/module.cue not found - run 'pudl init' first")
	}

	// Read and display module.cue content
	content, err := os.ReadFile(modulePath)
	if err != nil {
		return errors.NewFileNotFoundError("Failed to read module.cue")
	}

	fmt.Fprintln(outw(), "CUE Module Configuration:")
	fmt.Fprintln(outw(), "========================")
	fmt.Fprintf(outw(), "%s\n", content)

	return nil
}

func runModuleInfoCommand(ctx context.Context) error {
	// Load configuration to get schema path
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Check if module.cue exists
	schemaPath := effectiveSchemaPath(cfg)
	modulePath := filepath.Join(schemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError("cue.mod/module.cue not found - run 'pudl init' first")
	}

	fmt.Fprintf(outw(), "Module Information:\n")
	fmt.Fprintf(outw(), "==================\n")
	fmt.Fprintf(outw(), "Schema Directory: %s\n", effectiveSchemaPath(cfg))
	fmt.Fprintf(outw(), "Module File: %s\n", modulePath)

	// Show additional module information if CUE is available
	if requireCue() == nil {
		fmt.Fprintln(outw(), "\nModule Dependencies:")
		fmt.Fprintln(outw(), "===================")

		// Try to show module dependencies using cue mod edit
		cmd := cueCommand(ctx, effectiveSchemaPath(cfg), "mod", "edit", "--json")
		if output, err := cmd.Output(); err == nil {
			fmt.Fprintf(outw(), "%s\n", output)
		} else {
			fmt.Fprintln(outw(), "No dependencies or unable to read module information")
		}
	} else {
		fmt.Fprintln(outw(), "\n⚠️  CUE command not available - install from https://cuelang.org/docs/install/")
	}

	return nil
}

func runModuleAddCommand(ctx context.Context, moduleSpec string) error {
	// Load configuration to get schema path
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	if err := requireCue(); err != nil {
		return err
	}

	// Check if module.cue exists
	schemaPath := effectiveSchemaPath(cfg)
	modulePath := filepath.Join(schemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return errors.NewFileNotFoundError("cue.mod/module.cue not found - run 'pudl init' first")
	}

	fmt.Fprintf(outw(), "Adding module dependency: %s\n", moduleSpec)

	// Use cue mod get to add the dependency
	cmd := cueCommand(ctx, schemaPath, "mod", "get", moduleSpec)
	cmd.Stdout = outw()
	cmd.Stderr = errw()

	if err := cmd.Run(); err != nil {
		return errors.NewSystemError("Failed to add module dependency", err)
	}

	fmt.Fprintf(outw(), "✅ Module dependency %s added successfully\n", moduleSpec)
	fmt.Fprintln(outw(), "You can now import packages from this module in your CUE files.")

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
