package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/drift"
	"pudl/internal/mubridge"
)

var (
	exportActionsAll bool
)

var exportActionsCmd = &cobra.Command{
	Use:   "export-actions [--definition <name> | --all]",
	Short: "Export drift reports as mu-compatible action specs",
	Long: `Read drift reports and emit mu-compatible JSON action specs to stdout.

This bridges pudl's drift knowledge to mu's execution engine. Each field
difference in a drift report becomes an ActionSpec in the plan response.

Examples:
    pudl export-actions --definition my_instance
    pudl export-actions --all`,
	RunE: func(cmd *cobra.Command, args []string) error {
		defFlag, _ := cmd.Flags().GetString("definition")

		if !exportActionsAll && defFlag == "" {
			return fmt.Errorf("either --definition or --all is required")
		}
		if exportActionsAll && defFlag != "" {
			return fmt.Errorf("--definition and --all are mutually exclusive")
		}

		if exportActionsAll {
			return runExportActionsAll()
		}
		return runExportActions(defFlag)
	},
}

func init() {
	rootCmd.AddCommand(exportActionsCmd)
	exportActionsCmd.Flags().String("definition", "", "Definition name to export actions for")
	exportActionsCmd.Flags().BoolVar(&exportActionsAll, "all", false, "Export actions for all definitions with drift reports")
}

func runExportActions(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store := drift.NewReportStore(cfg.DataPath)
	report, err := store.GetLatest(name)
	if err != nil {
		return err
	}

	resp := mubridge.ExportFromDriftReport(report)
	return outputPlanResponse(resp)
}

func runExportActionsAll() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store := drift.NewReportStore(cfg.DataPath)
	defNames, err := store.ListDefinitions()
	if err != nil {
		return err
	}

	if len(defNames) == 0 {
		fmt.Fprintln(os.Stderr, "No definitions with drift reports found.")
		return outputPlanResponse(&mubridge.PlanResponse{})
	}

	// Merge all drift reports into a single plan response
	var allActions []mubridge.ActionSpec
	outputs := map[string]string{}

	for _, name := range defNames {
		report, err := store.GetLatest(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", name, err)
			continue
		}

		resp := mubridge.ExportFromDriftReport(report)
		allActions = append(allActions, resp.Actions...)
		for k, v := range resp.Outputs {
			outputs[name+"."+k] = v
		}
	}

	combined := &mubridge.PlanResponse{
		Actions: allActions,
		Outputs: outputs,
	}
	return outputPlanResponse(combined)
}

func outputPlanResponse(resp *mubridge.PlanResponse) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to encode plan response: %w", err)
	}
	return nil
}
