package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/vault"
)

var vaultGetCmd = &cobra.Command{
	Use:   "get <path>",
	Short: "Retrieve a secret from the vault",
	Long: `Retrieve a secret by its path.

Examples:
    pudl vault get my/api/key
    pudl vault get db/password`,
	Args: cobra.ExactArgs(1),
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

		val, err := v.Get(args[0])
		if err != nil {
			return err
		}

		fmt.Println(val)
		return nil
	},
}

func init() {
	vaultCmd.AddCommand(vaultGetCmd)
}
