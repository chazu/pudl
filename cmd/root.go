package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	pudlInit "pudl/internal/init"
	"pudl/internal/ui"
)

var (
	// Version information
	version = "dev"
	commit  = "unknown"
	date    = "unknown"

	// Global output flags
	jsonOutput bool
)

// GetOutputWriter returns an OutputWriter based on global flags
func GetOutputWriter() *ui.OutputWriter {
	format := ui.OutputFormatText
	if jsonOutput {
		format = ui.OutputFormatJSON
	}
	return ui.NewOutputWriter(format, true)
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "pudl",
	Short: "Personal Unified Data Lake - A tool for managing cloud infrastructure data",
	Long: `PUDL is a CLI tool that helps those who work with cloud resources
amplify their ability to leverage data as part of their regular workflows.

It manages the import, querying, and organization of a local 'data lake'
comprising data on remote resources such as AWS or GCP resources, Kubernetes
resources, logs, metrics, et cetera.

Key features:
- Schema management using CUE Lang
- Automatic CUE-based schema inference with cascade validation
- Version-controlled schema repository with git integration
- Data import from multiple sources and formats (JSON, YAML, CSV, NDJSON)
- Schema generation from imported data`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// Perform auto-initialization before executing any command
	// Skip auto-init for help, version, and init commands
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		if cmd != "help" && cmd != "version" && cmd != "init" && cmd != "--help" && cmd != "-h" && cmd != "--version" && cmd != "-v" {
			if err := pudlInit.AutoInitialize(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to auto-initialize PUDL workspace: %v\n", err)
				fmt.Fprintf(os.Stderr, "You may need to run 'pudl init' manually.\n")
			}
		}
	}

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// Global --json flag for machine-readable output
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output results as JSON for scripting")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("version", "v", false, "Show version information")
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  `Display version, commit hash, and build date information for PUDL.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("PUDL %s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built: %s\n", date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
