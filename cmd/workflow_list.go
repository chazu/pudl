package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/workflow"
)

var workflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available workflows",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		disc := workflow.NewDiscoverer(cfg.SchemaPath)
		workflows, err := disc.ListWorkflows()
		if err != nil {
			return fmt.Errorf("failed to list workflows: %w", err)
		}

		if len(workflows) == 0 {
			fmt.Println("No workflows found.")
			return nil
		}

		fmt.Printf("%-25s %-40s %s\n", "NAME", "DESCRIPTION", "STEPS")
		fmt.Printf("%-25s %-40s %s\n", "----", "-----------", "-----")
		for _, wf := range workflows {
			fmt.Printf("%-25s %-40s %d\n", wf.Name, wf.Description, len(wf.Steps))
		}

		return nil
	},
}

func init() {
	workflowCmd.AddCommand(workflowListCmd)
}
