package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/vault"
)

var vaultListCmd = &cobra.Command{
	Use:   "list",
	Short: "List stored secret paths",
	Long: `List all secret paths in the vault.

Examples:
    pudl vault list`,
	Args: cobra.NoArgs,
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

		paths, err := v.List()
		if err != nil {
			return err
		}

		if len(paths) == 0 {
			fmt.Println("No secrets stored")
			return nil
		}

		for _, p := range paths {
			fmt.Println(p)
		}
		return nil
	},
}

func init() {
	vaultCmd.AddCommand(vaultListCmd)
}
