package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
)

var modelDepsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Refresh and show the cross-model dependency graph",
	Long: `Reconcile every registered model's declared and binding-derived dependencies into model_depends_on
facts WITHOUT running the models, then print the dependency graph.

This closes the run-time-only coverage gap: querying impact (impacted_by) is
otherwise blind to models that have never been run. 'pudl model deps' records
every declared edge from the schema directly.

Examples:
    pudl model deps
    pudl model deps --json`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		models, _, err := listModels()
		if err != nil {
			return err
		}

		db, err := database.NewCatalogDB(effectivePudlDir())
		if err != nil {
			return fmt.Errorf("open catalog: %w", err)
		}
		defer db.Close()

		var warnings []string

		// 1. Declared and authoritative binding edges for every retained model
		// template (no successful value resolution or run needed).
		for _, model := range models {
			declared, warns := declaredDepsOfTemplate(model.Template)
			warnings = append(warnings, warns...)
			if rerr := reconcileEdges(db, model.Name, declaredSource(model.Name), declared); rerr != nil {
				return fmt.Errorf("reconcile declared deps for %s: %w", model.Name, rerr)
			}
			if rerr := reconcileBindingDependencies(db, model.Template); rerr != nil {
				return fmt.Errorf("reconcile binding deps for %s: %w", model.Name, rerr)
			}
		}

		return printDepGraph(db, warnings)
	},
}

// depEdge is one model_depends_on edge with its provenance for display.
type depEdge struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Sources []string `json:"sources"`
}

// printDepGraph reads the current model_depends_on facts and prints them grouped
// by model, annotated declared/derived.
func printDepGraph(db *database.CatalogDB, warnings []string) error {
	facts, err := db.QueryFacts(database.FactFilter{Relation: modelDependsRelation})
	if err != nil {
		return err
	}
	edgesByPair := map[string]*depEdge{}
	for _, f := range facts {
		from, to := edgeArgs(f.Args)
		if from == "" || to == "" {
			continue
		}
		key := from + "\x00" + to
		edge := edgesByPair[key]
		if edge == nil {
			edge = &depEdge{From: from, To: to}
			edgesByPair[key] = edge
		}
		label := sourceLabel(f.Source)
		if !containsString(edge.Sources, label) {
			edge.Sources = append(edge.Sources, label)
		}
	}
	edges := make([]depEdge, 0, len(edgesByPair))
	for _, edge := range edgesByPair {
		sort.Strings(edge.Sources)
		edges = append(edges, *edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	if jsonOutput {
		out := map[string]any{"edges": edges}
		if len(warnings) > 0 {
			out["warnings"] = warnings
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(outw(), string(data))
		return nil
	}

	for _, w := range warnings {
		fmt.Fprintf(errw(), "warning: %s\n", w)
	}
	if len(edges) == 0 {
		fmt.Fprintln(outw(), "No cross-model dependencies recorded.")
		return nil
	}
	fmt.Fprintf(outw(), "Cross-model dependencies (%d edge(s)):\n\n", len(edges))
	var lastFrom string
	for _, e := range edges {
		if e.From != lastFrom {
			fmt.Fprintf(outw(), "  %s depends on:\n", e.From)
			lastFrom = e.From
		}
		fmt.Fprintf(outw(), "    → %s  [%s]\n", e.To, strings.Join(e.Sources, ", "))
	}
	fmt.Fprintln(outw(), "\nQuery: pudl query depends_transitive from=<model> | impacted_by changed=<model> | --topo model_depends_on")
	return nil
}

// sourceLabel maps a fact source to a short provenance label.
func sourceLabel(source string) string {
	switch {
	case len(source) >= 6 && source[:6] == "model:":
		return "declared"
	case len(source) >= 8 && source[:8] == "derived:":
		return "derived"
	case len(source) >= 8 && source[:8] == "binding:":
		return "binding"
	default:
		return source
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func init() {
	modelCmd.AddCommand(modelDepsCmd)
}
