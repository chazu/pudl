package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/systemmodel"
)

type modelDescription struct {
	Name      string           `json:"name"`
	Populate  map[string]any   `json:"populate"`
	Desired   []map[string]any `json:"desired,omitempty"`
	Checks    []any            `json:"checks,omitempty"`
	DependsOn []string         `json:"depends_on,omitempty"`
	Plugins   []any            `json:"plugins,omitempty"`
	Converge  map[string]any   `json:"converge,omitempty"`
	Freshness map[string]any   `json:"freshness,omitempty"`
}

type muPluginDiscovery func(string) (map[string]any, error)

func describeSystemModel(m *systemmodel.SystemModel, discover muPluginDiscovery) modelDescription {
	d := modelDescription{
		Name:      m.Name,
		Populate:  map[string]any{"kind": string(m.Populate.Kind()), "plugin": m.Populate.Plugin, "input": m.Populate.Input, "differential": m.Populate.Differential},
		Desired:   m.Desired,
		DependsOn: m.DependsOn,
	}
	for _, p := range m.Plugins {
		entry := map[string]any{"name": p.Name, "source": p}
		if discover != nil {
			if info, infoErr := discover(p.Name); infoErr == nil {
				entry["discovery"] = info
			} else {
				entry["discovery_error"] = infoErr.Error()
			}
		}
		d.Plugins = append(d.Plugins, entry)
	}
	for _, c := range m.Checks {
		d.Checks = append(d.Checks, c)
	}
	if m.Convergent() {
		d.Converge = map[string]any{"plugin": m.Converge.Plugin, "input": m.Converge.Input}
	}
	if m.Freshness != nil {
		d.Freshness = map[string]any{"every": m.Freshness.Every, "drift": m.Freshness.Drift}
	}
	return d
}

var modelDescribeCmd = &cobra.Command{
	Use:               "describe <name>",
	Short:             "Describe a registered model for agents",
	Args:              cobra.ExactArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: completeModelNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, _, _, err := resolveModel(args[0])
		if err != nil {
			return err
		}
		d := describeSystemModel(m, loadMuPluginInfo)
		if jsonOutput {
			b, err := json.MarshalIndent(d, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		fmt.Printf("Model: %s\n", d.Name)
		fmt.Printf("  populate: %s (%s)\n", d.Populate["kind"], d.Populate["plugin"])
		fmt.Printf("  desired:  %d\n", len(d.Desired))
		fmt.Printf("  checks:   %d\n", len(d.Checks))
		if len(d.DependsOn) > 0 {
			fmt.Printf("  depends:  %s\n", strings.Join(d.DependsOn, ", "))
		}
		return nil
	},
}

func init() {
	modelCmd.AddCommand(modelDescribeCmd)
}
