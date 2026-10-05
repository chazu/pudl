package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/errors"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and manage PUDL configuration",
	Long: `View and manage PUDL configuration settings.

This command allows you to view the current configuration and see where
your PUDL workspace is located.

The configuration includes:
- Schema repository path (where CUE schemas are stored)
- Data directory path (where imported data is stored)
- Configuration file location

Example usage:
    pudl config                  # Show current configuration
    pudl config --path           # Show configuration file path
    pudl config set <key> <value> # Set a configuration value
    pudl config reset            # Reset to default configuration`,
	RunE: pudlRunE(runConfigCommand),
}

// runConfigCommand contains the actual config logic with structured error handling
func runConfigCommand(cmd *cobra.Command, args []string) error {
	showPath, _ := cmd.Flags().GetBool("path")

	if showPath {
		if jsonOutput {
			return printJSON(map[string]string{"config_file": config.ConfigPath(effectivePudlDir())})
		}
		fmt.Fprintln(outw(), config.ConfigPath(effectivePudlDir()))
		return nil
	}

	// Load and display configuration
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return err // Already a PUDLError from config.Load()
	}

	initialized := config.ExistsAt(effectivePudlDir())
	if jsonOutput {
		if !initialized {
			fmt.Fprintln(errw(), "⚠️  Workspace not initialized. Run 'pudl init' to set up.")
		}
		return writeConfigJSON(cfg)
	}

	fmt.Fprintln(outw(), "PUDL Configuration:")
	fmt.Fprintf(outw(), "  Workspace: %s\n", effectivePudlDir())
	fmt.Fprintf(outw(), "  Schema Path: %s\n", cfg.SchemaPath)
	if paths := effectiveSchemaPaths(cfg); len(paths) > 0 {
		fmt.Fprintf(outw(), "  Schema Search Paths: %s\n", strings.Join(paths, ", "))
	}
	fmt.Fprintf(outw(), "  Data Path: %s\n", cfg.DataPath)
	fmt.Fprintf(outw(), "  Config File: %s\n", config.ConfigPath(effectivePudlDir()))
	fmt.Fprintf(outw(), "  Version: %s\n", cfg.Version)

	// Check if workspace exists
	if !initialized {
		fmt.Fprintln(outw())
		fmt.Fprintln(errw(), "⚠️  Workspace not initialized. Run 'pudl init' to set up.")
	}

	return nil
}

// configSetCmd represents the config set command
var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value and save it to the configuration file.

Valid configuration keys:
- schema_path: Path to the schema repository directory
- data_path: Path to the data storage directory
- version: Configuration version

Inside a repository workspace, schema_path and data_path are fixed beneath
.pudl/ and cannot be redirected outside the repository-local state boundary.

Example usage:
    pudl config set schema_path ~/my-schemas
    pudl config set data_path /tmp/pudl-data
    pudl config set version 2.0`,
	Args: cobra.ExactArgs(2),
	RunE: pudlRunE(runConfigSetCommand),
}

// runConfigSetCommand contains the actual config set logic with structured error handling
func runConfigSetCommand(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]
	if wsPolicy != nil && wsPolicy.InWorkspace() && (key == "schema_path" || key == "data_path") {
		return errors.NewInputError(
			fmt.Sprintf("%s is fixed inside a repository workspace", key),
			fmt.Sprintf("Repository state must remain beneath %s", effectivePudlDir()),
			"Use `pudl config reset` to restore the local paths, or run the command outside a repository for global configuration",
		)
	}

	if err := config.SetConfigValueAt(effectivePudlDir(), key, value); err != nil {
		return err // Already a PUDLError from config.SetConfigValue()
	}

	// Show the updated configuration
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return err // Already a PUDLError from config.Load()
	}
	if jsonOutput {
		return writeConfigJSON(cfg)
	}

	fmt.Fprintf(outw(), "✅ Configuration updated: %s = %s\n", key, value)

	fmt.Fprintln(outw(), "Updated PUDL Configuration:")
	fmt.Fprintf(outw(), "  Schema Path: %s\n", cfg.SchemaPath)
	fmt.Fprintf(outw(), "  Data Path: %s\n", cfg.DataPath)
	fmt.Fprintf(outw(), "  Version: %s\n", cfg.Version)

	return nil
}

// configResetCmd represents the config reset command
var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration to defaults",
	Long: `Reset the PUDL configuration to default values.

This will restore:
- Schema path beneath the active .pudl workspace
- Data path beneath the active .pudl workspace
- Version to 1.0

Example usage:
    pudl config reset`,
	RunE: pudlRunE(runConfigResetCommand),
}

// runConfigResetCommand contains the actual config reset logic with structured error handling
func runConfigResetCommand(cmd *cobra.Command, args []string) error {
	if err := config.ResetToDefaultsAt(effectivePudlDir()); err != nil {
		return err // Already a PUDLError from config.ResetToDefaults()
	}

	// Show the reset configuration
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return err // Already a PUDLError from config.Load()
	}
	if jsonOutput {
		return writeConfigJSON(cfg)
	}

	fmt.Fprintln(outw(), "✅ Configuration reset to defaults")
	fmt.Fprintln(outw())

	fmt.Fprintln(outw(), "Reset PUDL Configuration:")
	fmt.Fprintf(outw(), "  Schema Path: %s\n", cfg.SchemaPath)
	fmt.Fprintf(outw(), "  Data Path: %s\n", cfg.DataPath)
	fmt.Fprintf(outw(), "  Version: %s\n", cfg.Version)

	return nil
}

func init() {
	rootCmd.AddCommand(configCmd)

	// Add subcommands
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configResetCmd)

	// Add flags
	configCmd.Flags().BoolP("path", "p", false, "Show configuration file path only")

	// Add help for valid keys
	configSetCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(outw(), "Set a configuration value\n\n")
		fmt.Fprintf(outw(), "Usage:\n  %s\n\n", cmd.UseLine())
		fmt.Fprintf(outw(), "Valid configuration keys:\n")
		for _, key := range config.ValidConfigKeys() {
			fmt.Fprintf(outw(), "  %s\n", key)
		}
		fmt.Fprintf(outw(), "\nExamples:\n")
		fmt.Fprintf(outw(), "  pudl config set schema_path ~/my-schemas\n")
		fmt.Fprintf(outw(), "  pudl config set data_path /tmp/pudl-data\n")
		fmt.Fprintf(outw(), "  pudl config set version 2.0\n")
	})
}

// writeConfigJSON writes the effective configuration as one JSON document.
func writeConfigJSON(cfg *config.Config) error {
	paths := effectiveSchemaPaths(cfg)
	if paths == nil {
		paths = []string{}
	}
	return printJSON(map[string]any{
		"workspace":           effectivePudlDir(),
		"schema_path":         cfg.SchemaPath,
		"schema_search_paths": paths,
		"data_path":           cfg.DataPath,
		"config_file":         config.ConfigPath(effectivePudlDir()),
		"version":             cfg.Version,
		"initialized":         config.ExistsAt(effectivePudlDir()),
	})
}
