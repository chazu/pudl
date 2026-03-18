package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/workflow"
)

var workflowValidateCmd = &cobra.Command{
	Use:   "validate <name>",
	Short: "Validate a workflow definition",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		disc := workflow.NewDiscoverer(cfg.SchemaPath)
		wf, err := disc.GetWorkflow(args[0])
		if err != nil {
			return err
		}

		// Build DAG (validates references)
		dag, err := workflow.BuildDAG(wf)
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}

		// Check for cycles
		order, err := dag.TopologicalSort()
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}

		// Check that all steps have definition and method
		for name, step := range wf.Steps {
			if step.Definition == "" {
				return fmt.Errorf("step %q missing definition", name)
			}
			if step.Method == "" {
				return fmt.Errorf("step %q missing method", name)
			}
		}

		fmt.Printf("Workflow %q is valid.\n", wf.Name)
		fmt.Printf("  Steps: %d\n", len(wf.Steps))
		fmt.Printf("  Execution order: %v\n", order)

		return nil
	},
}

func init() {
	workflowCmd.AddCommand(workflowValidateCmd)
}
