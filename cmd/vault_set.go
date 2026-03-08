package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/vault"
)

var vaultSetCmd = &cobra.Command{
	Use:   "set <path> <value>",
	Short: "Store a secret in the vault",
	Long: `Store a secret at the given path. Works with all backends.

For the env backend, this sets the environment variable for the current process only.
For the file backend, the secret is persisted to the encrypted vault file.

Examples:
    pudl vault set my/api/key sk-abc123
    pudl vault set db/password hunter2`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		v, err := vault.New(cfg.VaultBackend, config.GetPudlDir())
		if err != nil {
			return fmt.Errorf("failed to open vault: %w", err)
		}
		defer v.Close()

		if err := v.Set(args[0], args[1]); err != nil {
			return err
		}

		fmt.Printf("Secret %q stored successfully\n", args[0])
		return nil
	},
}

func init() {
	vaultCmd.AddCommand(vaultSetCmd)
}
