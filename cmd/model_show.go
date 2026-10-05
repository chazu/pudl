package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
)

var modelShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a registered #SystemModel definition",
	Long: `Show the details of a registered #SystemModel by name (the instance's
'name:' field or its short definition name): populate arm, desired state,
converge arm, checks, and declared plugins.`,
	Args:              cobra.ExactArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: completeModelNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, _, _, err := resolveModel(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			view := modelInspection{SystemModel: m}
			if modelDiscover {
				view.PluginDiscovery = discoverModelPlugins(m, muPluginInfoFrom(cmd.Context()))
			}
			b, err := json.MarshalIndent(view, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(outw(), string(b))
			return nil
		}
		cfg, err := loadEffectiveConfig()
		if err != nil {
			return err
		}
		schemas, err := validator.NewChainValidator(effectiveSchemaPaths(cfg)...)
		if err != nil {
			return fmt.Errorf("load schemas: %w", err)
		}
		printModel(m, schemas)
		if modelDiscover {
			for _, plugin := range discoverModelPlugins(m, muPluginInfoFrom(cmd.Context())) {
				fmt.Fprintf(outw(), "  Discovery: %s", plugin.Name)
				if plugin.Error != "" {
					fmt.Fprintf(outw(), " (%s)", plugin.Error)
				}
				fmt.Fprintln(outw())
			}
		}
		return nil
	},
}

func printModel(m *systemmodel.SystemModel, schemas *validator.ChainValidator) {
	fmt.Fprintf(outw(), "Model: %s\n", m.Name)
	fmt.Fprintln(outw(), strings.Repeat("-", 60))

	// Populate arm.
	switch m.Populate.Kind() {
	case systemmodel.KindEweTarget:
		fmt.Fprintf(outw(), "  Populate:  ewe (%s)\n", m.Populate.EweSource)
		if len(m.Populate.Outputs) > 0 {
			fmt.Fprintf(outw(), "    outputs: %s\n", strings.Join(m.Populate.Outputs, ", "))
		}
	default:
		fmt.Fprintf(outw(), "  Populate:  observe (plugin %q)\n", m.Populate.Plugin)
	}

	// Converge arm.
	if m.Convergent() {
		fmt.Fprintf(outw(), "  Converge:  %s\n", m.Converge.Plugin)
	} else {
		fmt.Fprintf(outw(), "  Converge:  (observe-only)\n")
	}

	// Desired state.
	fmt.Fprintf(outw(), "  Desired:   %d resource(s)\n", len(m.Desired))
	for _, d := range m.Desired {
		if s, ok := d["_schema"].(string); ok {
			fmt.Fprintf(outw(), "    - %s\n", describeDesiredSchema(s, schemas))
		}
	}

	// Checks.
	if len(m.Checks) > 0 {
		fmt.Fprintf(outw(), "  Checks:    %d\n", len(m.Checks))
		for _, c := range m.Checks {
			fmt.Fprintf(outw(), "    - %s (%s, expect %s)\n", c.Name, c.Severity, c.Expect)
		}
	}

	if len(m.DependsOn) > 0 {
		fmt.Fprintf(outw(), "  Depends:   %s\n", strings.Join(m.DependsOn, ", "))
	}
	if m.Freshness != nil {
		fmt.Fprintf(outw(), "  Freshness: every=%s drift=%t\n", m.Freshness.Every, m.Freshness.Drift)
	}

	// Plugins.
	if len(m.Plugins) > 0 {
		names := make([]string, 0, len(m.Plugins))
		for _, p := range m.Plugins {
			names = append(names, p.Name)
		}
		fmt.Fprintf(outw(), "  Plugins:   %s\n", strings.Join(names, ", "))
	}
}

// Desired records may carry a resource type instead of a CUE schema reference.
// Resolve through metadata, not a guessed package/definition naming convention.
func describeDesiredSchema(ref string, schemas *validator.ChainValidator) string {
	if strings.Contains(ref, ".#") || strings.Contains(ref, ":#") {
		name := schemaname.Normalize(ref)
		if schemas.HasSchema(name) {
			return name
		}
		return name + " (not registered)"
	}
	matches := schemas.GetSchemasByResourceType(ref)
	switch len(matches) {
	case 0:
		return fmt.Sprintf("resource type: %s (no registered schema)", ref)
	case 1:
		return fmt.Sprintf("%s (resource type: %s)", matches[0], ref)
	default:
		return fmt.Sprintf("resource type: %s (multiple schemas: %s)", ref, strings.Join(matches, ", "))
	}
}

// completeModelNames provides shell completion of registered model names.
func completeModelNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	models, _, err := listModels()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, mi := range models {
		if toComplete == "" || strings.HasPrefix(mi.Name, toComplete) {
			out = append(out, mi.Name+"\t"+mi.SchemaName)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	modelCmd.AddCommand(modelShowCmd)
	modelShowCmd.Flags().BoolVar(&modelDiscover, "discover", false, "Include live Mu plugin capability discovery")
}
