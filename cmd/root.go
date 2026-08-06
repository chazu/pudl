package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	pudlInit "github.com/chazu/pudl/internal/init"
	"github.com/chazu/pudl/internal/ui"
	"github.com/chazu/pudl/internal/workspace"
)

var (
	// Version information
	version = "dev"
	commit  = "unknown"
	date    = "unknown"

	// Global output flags
	jsonOutput bool

	// wsPolicy is every path decision this invocation makes — schema, definition,
	// rule, model and populator search paths, the effective origin, and whether
	// we are in a repo workspace. Resolved once here so no call site re-derives
	// it and no two derivations can disagree.
	wsPolicy *workspace.Policy
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
- Schema generation from imported data
- Observe-only and converging #SystemModel runs through mu
- Exact producer/consumer run-sets with strict sealed action routing and approval`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Resolve the workspace policy (global mode when no repo workspace).
		var err error
		wsPolicy, err = workspace.ResolveForCWD()
		if err != nil {
			return fmt.Errorf("workspace discovery: %w", err)
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// Perform global auto-initialization only outside a repository workspace.
	// A local workspace is self-contained; touching ~/.pudl before Cobra resolves
	// it would violate the repo-local persistence boundary.
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		cwd, _ := os.Getwd()
		localWorkspace, discoverErr := workspace.Discover(cwd)
		if discoverErr == nil && localWorkspace == nil && shouldAutoInitializeGlobal(cmd) {
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

func shouldAutoInitializeGlobal(command string) bool {
	switch command {
	case "help", "version", "init", "repo", "--help", "-h", "--version", "-v":
		return false
	default:
		return true
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
