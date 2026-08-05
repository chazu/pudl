package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/repo"
)

var repoInitForce bool

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Repository-wide operations",
	Long: `Operations that span the entire schema repository.

Available subcommands:
- init: Initialize PUDL in the current repository

Examples:
    pudl repo init`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var repoInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize PUDL in the current repository",
	Long: `Initialize a .pudl/ directory in the current repository and install
Claude skills into .claude/skills/.

This sets up the current repo for project-local PUDL usage, including:
- .pudl/workspace.cue and repo-local configuration
- .pudl/schema/ with a CUE module and every built-in schema
- .pudl/data/ for the local catalog, raw data, metadata, and run state
- .claude/skills/ with PUDL skill files for AI agent integration

The command is safe to repeat: it repairs PUDL-owned files while preserving
authored workspace configuration. Use --force to replace that configuration.

Examples:
    pudl repo init
    pudl repo init --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := repo.InitOptions{
			Force:   repoInitForce,
			Verbose: true,
		}
		if err := repo.Init(opts); err != nil {
			return errors.WrapError(errors.ErrCodeFileSystem, "repo init failed", err)
		}
		fmt.Println()
		fmt.Println("Repo initialized. Durable PUDL state will stay under .pudl/.")
		fmt.Println("PUDL skills are available in .claude/skills/.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(repoCmd)
	repoCmd.AddCommand(repoInitCmd)

	repoInitCmd.Flags().BoolVar(&repoInitForce, "force", false, "Force reinitialize existing repo")
}
