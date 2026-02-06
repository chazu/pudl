package cmd

import (
	"github.com/spf13/cobra"
)

var (
	schemaVerbose bool
	schemaPackage string
	commitMessage string
	logLimit      int
)

// schemaCmd represents the schema command
var schemaCmd = &cobra.Command{
	Use:     "schema",
	Aliases: []string{"s"},
	Short:   "Manage CUE schemas for data validation",
	Long: `Manage CUE schemas used for data validation and organization in PUDL.

Schemas are organized by packages (aws, k8s, unknown, etc.) and stored in the
schema repository at ~/.pudl/schema/. Each schema is a CUE file that defines
the structure and validation rules for imported data.

Available subcommands:
- list:    Show available schemas organized by package
- add:     Add a new schema file to the repository
- new:     Generate a new schema from imported data
- show:    Display the contents of a schema
- edit:    Open a schema file in your editor
- status:  Show uncommitted changes in the schema repository
- commit:  Commit schema changes to version control
- log:     Show commit history of schema changes
- reinfer: Re-run schema inference on existing entries
- migrate: Migrate schema names to canonical format

Examples:
    pudl schema list                           # List all schemas
    pudl schema list --package aws             # List schemas in aws package
    pudl schema add aws.rds-instance my.cue    # Add new schema to aws package
    pudl schema status                         # Show uncommitted changes
    pudl schema commit -m "Add RDS schema"     # Commit changes
    pudl schema log                            # Show recent commits`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: show help
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}
