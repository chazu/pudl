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
- .pudl/ directory (project-local marker)
- .claude/skills/ with PUDL skill files for AI agent integration

Use --force to reinitialize an existing repo.

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
		fmt.Println("Repo initialized. PUDL skills are available in .claude/skills/.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(repoCmd)
	repoCmd.AddCommand(repoInitCmd)

	repoInitCmd.Flags().BoolVar(&repoInitForce, "force", false, "Force reinitialize existing repo")
}
