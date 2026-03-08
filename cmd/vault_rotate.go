package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/vault"
)

var vaultRotateCmd = &cobra.Command{
	Use:   "rotate-key",
	Short: "Re-encrypt file vault with a new passphrase",
	Long: `Re-encrypt the file vault with a new passphrase.

The current passphrase must be set in PUDL_VAULT_PASSPHRASE.
The new passphrase must be set in PUDL_VAULT_NEW_PASSPHRASE.

Examples:
    PUDL_VAULT_PASSPHRASE=old PUDL_VAULT_NEW_PASSPHRASE=new pudl vault rotate-key`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if cfg.VaultBackend != "file" {
			return fmt.Errorf("rotate-key only works with the file vault backend (current: %q)", cfg.VaultBackend)
		}

		newPassphrase := os.Getenv("PUDL_VAULT_NEW_PASSPHRASE")
		if newPassphrase == "" {
			return fmt.Errorf("PUDL_VAULT_NEW_PASSPHRASE environment variable must be set")
		}

		v, err := vault.New("file", config.GetPudlDir())
		if err != nil {
			return fmt.Errorf("failed to open vault: %w", err)
		}
		defer v.Close()

		fv, ok := v.(*vault.FileVault)
		if !ok {
			return fmt.Errorf("rotate-key requires a file vault backend")
		}

		if err := fv.RotateKey(newPassphrase); err != nil {
			return fmt.Errorf("failed to rotate key: %w", err)
		}

		fmt.Println("Vault key rotated successfully")
		fmt.Println("Update PUDL_VAULT_PASSPHRASE to the new passphrase")
		return nil
	},
}

func init() {
	vaultCmd.AddCommand(vaultRotateCmd)
}
