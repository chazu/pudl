package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version information
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "pudl",
	Short: "Personal Unified Data Lake - A tool for managing cloud infrastructure data",
	Long: `PUDL is a CLI tool that helps those who work with cloud resources 
amplify their ability to leverage data as part of their regular workflows. 

It manages the import, querying, processing and updating of a local 'data lake' 
comprising data on remote resources such as AWS or GCP resources, Kubernetes 
resources, logs, metrics, et cetera.

Key features:
- Schema management using CUE Lang
- Automatic schema inference using embedded Lisp rules
- Version-controlled schema repository
- Data import from multiple sources and formats
- Outlier detection and infrastructure sprawl reduction`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.pudl.yaml)")

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
