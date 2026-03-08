package cmd

import (
	"github.com/spf13/cobra"
)

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Manage secrets in the vault",
	Long: `Manage secrets used by definitions and methods.

Available subcommands:
- get:        Retrieve a secret
- set:        Store a secret (file backend only)
- list:       List stored secret paths
- rotate-key: Re-encrypt file vault with new passphrase`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(vaultCmd)
}
