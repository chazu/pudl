package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/workflow"
)

var workflowHistoryCmd = &cobra.Command{
	Use:   "history <name>",
	Short: "Show past run history for a workflow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store := workflow.NewManifestStore(cfg.DataPath)
		ids, err := store.List(args[0])
		if err != nil {
			return fmt.Errorf("failed to list runs: %w", err)
		}

		if len(ids) == 0 {
			fmt.Printf("No runs found for workflow %q.\n", args[0])
			return nil
		}

		fmt.Printf("%-25s %-10s %-12s %s\n", "RUN ID", "STATUS", "DURATION", "STEPS")
		fmt.Printf("%-25s %-10s %-12s %s\n", "------", "------", "--------", "-----")

		for _, id := range ids {
			m, err := store.Get(args[0], id)
			if err != nil {
				continue
			}

			stepSummary := ""
			success, failed, skipped := 0, 0, 0
			for _, s := range m.Steps {
				switch s.Status {
				case "success":
					success++
				case "failed":
					failed++
				case "skipped":
					skipped++
				}
			}
			stepSummary = fmt.Sprintf("%d ok", success)
			if failed > 0 {
				stepSummary += fmt.Sprintf(", %d fail", failed)
			}
			if skipped > 0 {
				stepSummary += fmt.Sprintf(", %d skip", skipped)
			}

			fmt.Printf("%-25s %-10s %-12.2fs %s\n", m.RunID, m.Status, m.DurationSecs, stepSummary)
		}

		return nil
	},
}

func init() {
	workflowCmd.AddCommand(workflowHistoryCmd)
}
