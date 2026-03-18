package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/workflow"
)

var workflowShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show workflow details",
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

		fmt.Printf("Workflow: %s\n", wf.Name)
		if wf.Description != "" {
			fmt.Printf("Description: %s\n", wf.Description)
		}
		fmt.Printf("Abort on failure: %v\n", wf.AbortOnFailure)
		fmt.Printf("File: %s\n\n", wf.FilePath)

		// Build DAG for topo order
		dag, err := workflow.BuildDAG(wf)
		if err != nil {
			return fmt.Errorf("failed to build DAG: %w", err)
		}

		order, err := dag.TopologicalSort()
		if err != nil {
			return fmt.Errorf("invalid workflow: %w", err)
		}

		fmt.Println("Steps (topological order):")
		for i, name := range order {
			step := wf.Steps[name]
			fmt.Printf("  %d. %s\n", i+1, name)
			fmt.Printf("     definition: %s\n", step.Definition)
			fmt.Printf("     method:     %s\n", step.Method)

			deps := dag.GetDependencies(name)
			if len(deps) > 0 {
				fmt.Printf("     depends on: %v\n", deps)
			}
			if len(step.Inputs) > 0 {
				fmt.Printf("     inputs:\n")
				for k, v := range step.Inputs {
					fmt.Printf("       %s: %s\n", k, v)
				}
			}
			if step.Timeout > 0 {
				fmt.Printf("     timeout:    %s\n", step.Timeout)
			}
			if step.Retries > 0 {
				fmt.Printf("     retries:    %d\n", step.Retries)
			}
		}

		return nil
	},
}

func init() {
	workflowCmd.AddCommand(workflowShowCmd)
}
