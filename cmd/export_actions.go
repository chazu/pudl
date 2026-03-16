package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/definition"
	"pudl/internal/drift"
	"pudl/internal/mubridge"
)

var (
	exportActionsAll bool
)

var exportActionsCmd = &cobra.Command{
	Use:   "export-actions [--definition <name> | --all]",
	Short: "Export drift reports as a mu.json configuration",
	Long: `Read drift reports and emit a mu-compatible JSON config to stdout.

Each drifted definition becomes a mu target whose config is the desired state.
The toolchain is inferred from the definition's schema reference. mu plugins
for each resource type handle the actual convergence.

Examples:
    pudl export-actions --definition my_instance
    pudl export-actions --all
    pudl export-actions --all | mu build --config /dev/stdin //...`,
	RunE: func(cmd *cobra.Command, args []string) error {
		defFlag, _ := cmd.Flags().GetString("definition")

		if !exportActionsAll && defFlag == "" {
			return fmt.Errorf("either --definition or --all is required")
		}
		if exportActionsAll && defFlag != "" {
			return fmt.Errorf("--definition and --all are mutually exclusive")
		}

		if exportActionsAll {
			return runExportAll()
		}
		return runExportOne(defFlag)
	},
}

func init() {
	rootCmd.AddCommand(exportActionsCmd)
	exportActionsCmd.Flags().String("definition", "", "Definition name to export actions for")
	exportActionsCmd.Flags().BoolVar(&exportActionsAll, "all", false, "Export actions for all definitions with drift reports")
}

func runExportOne(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store := drift.NewReportStore(cfg.DataPath)
	report, err := store.GetLatest(name)
	if err != nil {
		return err
	}

	defDisc := definition.NewDiscoverer(cfg.SchemaPath)
	def, err := defDisc.GetDefinition(name)
	if err != nil {
		return fmt.Errorf("loading definition %q: %w", name, err)
	}

	inputs := []*mubridge.DriftInput{
		{
			Result:    report,
			SchemaRef: def.SchemaRef,
			Sources:   []string{def.FilePath},
		},
	}

	return outputMuConfig(mubridge.ExportMuConfig(inputs, nil))
}

func runExportAll() error {
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
		return outputMuConfig(&mubridge.MuConfig{})
	}

	defDisc := definition.NewDiscoverer(cfg.SchemaPath)

	var inputs []*mubridge.DriftInput
	for _, name := range defNames {
		report, err := store.GetLatest(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", name, err)
			continue
		}

		schemaRef := "generic"
		var sources []string
		if def, err := defDisc.GetDefinition(name); err == nil {
			schemaRef = def.SchemaRef
			sources = []string{def.FilePath}
		}

		inputs = append(inputs, &mubridge.DriftInput{
			Result:    report,
			SchemaRef: schemaRef,
			Sources:   sources,
		})
	}

	return outputMuConfig(mubridge.ExportMuConfig(inputs, nil))
}

func outputMuConfig(cfg *mubridge.MuConfig) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode mu config: %w", err)
	}
	return nil
}
