package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/errors"
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
and configuration files with proper CUE module support.

This command creates:
- ~/.pudl/ directory structure
- ~/.pudl/schema/ directory with CUE module and git repository
- ~/.pudl/data/ directory for data storage
- ~/.pudl/config.yaml configuration file

The schema directory is initialized as:
- A proper CUE module with cue.mod/module.cue
- A git repository for version control
- Local schema directories under pudl/
- Example files showing third-party module usage

This enables you to use third-party CUE modules like Kubernetes schemas
alongside your local PUDL schemas.

By default, this command will not overwrite an existing workspace. Use the
--force flag to reinitialize an existing workspace.

Example usage:
    pudl init                    # Initialize workspace with CUE module support
    pudl init --force            # Force reinitialize existing workspace`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create error handler for CLI context
		errorHandler := errors.NewCLIErrorHandler(true)

		// Run the init command and handle any errors
		if err := runInitCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

// runInitCommand contains the actual init logic with structured error handling
func runInitCommand(cmd *cobra.Command, args []string) error {
	// Check if already initialized
	if !initForce && config.Exists() {
		pudlDir := config.GetPudlDir()
		fmt.Printf("PUDL workspace already initialized at %s\n", pudlDir)
		fmt.Println("Use --force to reinitialize")
		return nil
	}

	// Perform initialization
	opts := pudlInit.InitOptions{
		Force:   initForce,
		Verbose: true,
	}

	if err := pudlInit.Initialize(opts); err != nil {
		return errors.WrapError(errors.ErrCodeFileSystem, "Failed to initialize PUDL workspace", err)
	}

	// Show next steps
	fmt.Println()
	fmt.Println("🚀 Next steps:")
	fmt.Println("   1. Import some data: pudl import --path <file>")
	fmt.Println("   2. List your data: pudl list")
	fmt.Println("   3. Check the examples: cat ~/.pudl/schema/examples/kubernetes.cue")
	fmt.Println("   4. Add third-party modules: pudl module add <module@version>")
	fmt.Println("   5. Process CUE files: pudl process <file.cue>")
	fmt.Println()
	fmt.Println("📚 Module management:")
	fmt.Println("   - List dependencies: pudl module list")
	fmt.Println("   - Update dependencies: pudl module tidy")
	fmt.Println("   - Module information: pudl module info")
	fmt.Println()
	fmt.Println("For help with any command, use: pudl <command> --help")

	return nil
}

func init() {
	rootCmd.AddCommand(initCmd)

	// Add flags
	initCmd.Flags().BoolVar(&initForce, "force", false, "Force reinitialize existing workspace")
}
