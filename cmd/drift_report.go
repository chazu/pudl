package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/drift"
)

var driftReportCmd = &cobra.Command{
	Use:   "report <definition>",
	Short: "Show saved drift reports",
	Long: `Display the latest drift report for a definition without re-running detection.

Examples:
    pudl drift report my_instance`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDriftReport(args[0])
	},
}

func init() {
	driftCmd.AddCommand(driftReportCmd)
}

func runDriftReport(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store := drift.NewReportStore(cfg.DataPath)
	result, err := store.GetLatest(name)
	if err != nil {
		return err
	}

	printDriftResult(result)

	// Show report history
	ids, err := store.List(name)
	if err != nil {
		return err
	}

	if len(ids) > 1 {
		fmt.Printf("\nHistory (%d reports):\n", len(ids))
		for i, id := range ids {
			if i >= 10 {
				fmt.Printf("  ... and %d more\n", len(ids)-10)
				break
			}
			fmt.Printf("  %s\n", id)
		}
	}

	return nil
}
