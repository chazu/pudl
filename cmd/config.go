package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
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
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the config command and handle any errors
		if err := runConfigCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runConfigCommand contains the actual config logic with structured error handling
func runConfigCommand(cmd *cobra.Command, args []string) error {
	showPath, _ := cmd.Flags().GetBool("path")

	if showPath {
		fmt.Println(config.GetConfigPath())
		return nil
	}

	// Load and display configuration
	cfg, err := config.Load()
	if err != nil {
		return err // Already a PUDLError from config.Load()
	}

	fmt.Println("PUDL Configuration:")
	fmt.Printf("  Workspace: %s\n", config.GetPudlDir())
	fmt.Printf("  Schema Path: %s\n", cfg.SchemaPath)
	fmt.Printf("  Data Path: %s\n", cfg.DataPath)
	fmt.Printf("  Config File: %s\n", config.GetConfigPath())
	fmt.Printf("  Version: %s\n", cfg.Version)

	// Check if workspace exists
	if !config.Exists() {
		fmt.Println()
		fmt.Println("⚠️  Workspace not initialized. Run 'pudl init' to set up.")
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

Example usage:
    pudl config set schema_path ~/my-schemas
    pudl config set data_path /tmp/pudl-data
    pudl config set version 2.0`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the config set command and handle any errors
		if err := runConfigSetCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runConfigSetCommand contains the actual config set logic with structured error handling
func runConfigSetCommand(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	if err := config.SetConfigValue(key, value); err != nil {
		return err // Already a PUDLError from config.SetConfigValue()
	}

	fmt.Printf("✅ Configuration updated: %s = %s\n", key, value)

	// Show the updated configuration
	cfg, err := config.Load()
	if err != nil {
		return err // Already a PUDLError from config.Load()
	}

	fmt.Println("Updated PUDL Configuration:")
	fmt.Printf("  Schema Path: %s\n", cfg.SchemaPath)
	fmt.Printf("  Data Path: %s\n", cfg.DataPath)
	fmt.Printf("  Version: %s\n", cfg.Version)

	return nil
}

// configResetCmd represents the config reset command
var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration to defaults",
	Long: `Reset the PUDL configuration to default values.

This will restore:
- Schema path to ~/.pudl/schema
- Data path to ~/.pudl/data
- Version to 1.0

Example usage:
    pudl config reset`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the config reset command and handle any errors
		if err := runConfigResetCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runConfigResetCommand contains the actual config reset logic with structured error handling
func runConfigResetCommand(cmd *cobra.Command, args []string) error {
	if err := config.ResetToDefaults(); err != nil {
		return err // Already a PUDLError from config.ResetToDefaults()
	}

	fmt.Println("✅ Configuration reset to defaults")

	// Show the reset configuration
	fmt.Println()
	cfg, err := config.Load()
	if err != nil {
		return err // Already a PUDLError from config.Load()
	}

	fmt.Println("Reset PUDL Configuration:")
	fmt.Printf("  Schema Path: %s\n", cfg.SchemaPath)
	fmt.Printf("  Data Path: %s\n", cfg.DataPath)
	fmt.Printf("  Version: %s\n", cfg.Version)

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
		fmt.Printf("Set a configuration value\n\n")
		fmt.Printf("Usage:\n  %s\n\n", cmd.UseLine())
		fmt.Printf("Valid configuration keys:\n")
		for _, key := range config.ValidConfigKeys() {
			fmt.Printf("  %s\n", key)
		}
		fmt.Printf("\nExamples:\n")
		fmt.Printf("  pudl config set schema_path ~/my-schemas\n")
		fmt.Printf("  pudl config set data_path /tmp/pudl-data\n")
		fmt.Printf("  pudl config set version 2.0\n")
	})
}
