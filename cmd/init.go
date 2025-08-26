package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	pudlInit "pudl/internal/init"
)

var (
	initForce bool
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize PUDL workspace",
	Long: `Initialize the PUDL workspace by creating the necessary directory structure
and configuration files.

This command creates:
- ~/.pudl/ directory structure
- ~/.pudl/schema/ directory with git repository for schema version control
- ~/.pudl/data/ directory for data storage
- ~/.pudl/config.yaml configuration file

The schema directory is initialized as a git repository with a README and
.gitignore file to help you get started with schema management.

By default, this command will not overwrite an existing workspace. Use the
--force flag to reinitialize an existing workspace.

Example usage:
    pudl init                    # Initialize workspace
    pudl init --force            # Force reinitialize existing workspace`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if already initialized
		if !initForce && config.Exists() {
			pudlDir := config.GetPudlDir()
			fmt.Printf("PUDL workspace already initialized at %s\n", pudlDir)
			fmt.Println("Use --force to reinitialize")
			return
		}

		// Perform initialization
		opts := pudlInit.InitOptions{
			Force:   initForce,
			Verbose: true,
		}

		if err := pudlInit.Initialize(opts); err != nil {
			log.Fatalf("Failed to initialize PUDL workspace: %v", err)
		}

		// Show next steps
		fmt.Println()
		fmt.Println("🚀 Next steps:")
		fmt.Println("   1. Import some data: pudl import --path <file>")
		fmt.Println("   2. List your data: pudl list")
		fmt.Println("   3. Process CUE files: pudl process <file.cue>")
		fmt.Println()
		fmt.Println("For help with any command, use: pudl <command> --help")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	// Add flags
	initCmd.Flags().BoolVar(&initForce, "force", false, "Force reinitialize existing workspace")
}
