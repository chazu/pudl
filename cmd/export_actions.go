package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
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
		buildDriftInput(def, report),
	}

	if err := outputMuConfig(mubridge.ExportMuConfig(inputs, loadMappings(cfg))); err != nil {
		return err
	}

	// Mark drifted definitions as converging
	if err := markConverging(cfg, inputs); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update status: %v\n", err)
	}

	return nil
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
		var def *definition.DefinitionInfo
		if d, err := defDisc.GetDefinition(name); err == nil {
			def = d
			schemaRef = def.SchemaRef
			sources = []string{def.FilePath}
		}

		if def != nil {
			inputs = append(inputs, buildDriftInput(def, report))
		} else {
			inputs = append(inputs, &mubridge.DriftInput{
				Result:    report,
				SchemaRef: schemaRef,
				Sources:   sources,
			})
		}
	}

	if err := outputMuConfig(mubridge.ExportMuConfig(inputs, loadMappings(cfg))); err != nil {
		return err
	}

	// Mark drifted definitions as converging
	if err := markConverging(cfg, inputs); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update status: %v\n", err)
	}

	return nil
}

// markConverging opens the catalog DB and sets status to "converging" for
// exported definitions that are not clean.
func markConverging(cfg *config.Config, inputs []*mubridge.DriftInput) error {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return err
	}
	defer db.Close()

	for _, input := range inputs {
		if input.Result.Status != "clean" {
			if err := db.UpdateStatus(input.Result.Definition, "converging"); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to update status for %s: %v\n", input.Result.Definition, err)
			}
		}
	}
	return nil
}

// loadMappings merges user-configured toolchain mappings with defaults.
// User mappings take precedence (checked first).
func loadMappings(cfg *config.Config) []mubridge.ToolchainMapping {
	if len(cfg.ToolchainMappings) == 0 {
		return nil // nil triggers DefaultMappings in ExportMuConfig
	}
	// User mappings first (higher priority), then defaults.
	var merged []mubridge.ToolchainMapping
	for _, m := range cfg.ToolchainMappings {
		merged = append(merged, mubridge.ToolchainMapping{
			Prefix:    m.Prefix,
			Toolchain: m.Toolchain,
		})
	}
	merged = append(merged, mubridge.DefaultMappings...)
	return merged
}

// buildDriftInput creates a DriftInput from a definition and drift report,
// extracting BRICK-specific fields when the definition is a BRICK target.
func buildDriftInput(def *definition.DefinitionInfo, report *drift.DriftResult) *mubridge.DriftInput {
	input := &mubridge.DriftInput{
		Result:    report,
		SchemaRef: def.SchemaRef,
		Sources:   []string{def.FilePath},
	}

	// If this is a BRICK target, extract toolchain and config from declared state.
	if isBrickTarget(def.SchemaRef) {
		if tc, ok := report.DeclaredKeys["toolchain"]; ok {
			if tcStr, ok := tc.(string); ok {
				input.BrickToolchain = tcStr
			}
		}
		if configMap, ok := report.DeclaredKeys["config"]; ok {
			if cm, ok := configMap.(map[string]interface{}); ok {
				input.BrickConfig = cm
			}
		}
	}

	return input
}

// isBrickTarget checks if a schema ref is a BRICK target type.
func isBrickTarget(schemaRef string) bool {
	return strings.Contains(schemaRef, "brick.#Target") ||
		strings.Contains(schemaRef, "brick.#Kit") ||
		strings.Contains(schemaRef, "brick.#Interface")
}

func outputMuConfig(cfg *mubridge.MuConfig) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode mu config: %w", err)
	}
	return nil
}
