package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/systemmodel"
)

var modelShowJSON bool

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
		if modelShowJSON {
			b, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		printModel(m)
		return nil
	},
}

func printModel(m *systemmodel.SystemModel) {
	fmt.Printf("Model: %s\n", m.Name)
	fmt.Println(strings.Repeat("-", 60))

	// Populate arm.
	switch m.Populate.Kind() {
	case systemmodel.KindEweTarget:
		fmt.Printf("  Populate:  ewe (%s)\n", m.Populate.EweSource)
		if len(m.Populate.Outputs) > 0 {
			fmt.Printf("    outputs: %s\n", strings.Join(m.Populate.Outputs, ", "))
		}
	default:
		fmt.Printf("  Populate:  observe (plugin %q)\n", m.Populate.Plugin)
	}

	// Converge arm.
	if m.Convergent() {
		fmt.Printf("  Converge:  %s\n", m.Converge.Plugin)
	} else {
		fmt.Printf("  Converge:  (observe-only)\n")
	}

	// Desired state.
	fmt.Printf("  Desired:   %d definition(s)\n", len(m.Desired))
	for _, d := range m.Desired {
		if s, ok := d["_schema"].(string); ok {
			fmt.Printf("    - %s\n", s)
		}
	}

	// Checks.
	if len(m.Checks) > 0 {
		fmt.Printf("  Checks:    %d\n", len(m.Checks))
		for _, c := range m.Checks {
			fmt.Printf("    - %s (%s, expect %s)\n", c.Name, c.Severity, c.Expect)
		}
	}

	// Plugins.
	if len(m.Plugins) > 0 {
		names := make([]string, 0, len(m.Plugins))
		for _, p := range m.Plugins {
			names = append(names, p.Name)
		}
		fmt.Printf("  Plugins:   %s\n", strings.Join(names, ", "))
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
	modelShowCmd.Flags().BoolVar(&modelShowJSON, "json", false, "output as JSON")
}
