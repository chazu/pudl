package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

var modelDepsDerive bool

var modelDepsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Refresh and show the cross-model dependency graph",
	Long: `Reconcile every registered model's declared depends_on into model_depends_on
facts WITHOUT running the models, then print the dependency graph.

This closes the run-time-only coverage gap: querying impact (impacted_by) is
otherwise blind to models that have never been run. 'pudl model deps' records
every declared edge from the schema directly.

With --derive, also compute Phase-2 DERIVED edges: B depends on A when a value
in B's desired references an identity A produces (e.g. B's Deployment names a
Namespace A declares), without a manual depends_on. Derived edges are emitted as
the same model_depends_on relation under a separate provenance, are heuristic
(value-based matching can over-match), and never override a declared edge.

Examples:
    pudl model deps
    pudl model deps --derive
    pudl model deps --derive --json`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		models, _, err := listModels()
		if err != nil {
			return err
		}
		ms := make([]*systemmodel.SystemModel, 0, len(models))
		for _, mi := range models {
			ms = append(ms, mi.Model)
		}

		db, err := database.NewCatalogDB(config.GetPudlDir())
		if err != nil {
			return fmt.Errorf("open catalog: %w", err)
		}
		defer db.Close()

		var warnings []string

		// 1. Declared edges for every model (no run needed).
		for _, m := range ms {
			declared, warns := declaredDepsOf(m)
			warnings = append(warnings, warns...)
			if rerr := reconcileEdges(db, m.Name, declaredSource(m.Name), declared); rerr != nil {
				return fmt.Errorf("reconcile declared deps for %s: %w", m.Name, rerr)
			}
		}

		// 2. Derived edges (opt-in).
		if modelDepsDerive {
			identity, ierr := schemaIdentityResolver()
			if ierr != nil {
				return ierr
			}
			derived := deriveDependencies(ms, identity)
			for _, m := range ms {
				if rerr := reconcileEdges(db, m.Name, derivedSource(m.Name), derived[m.Name]); rerr != nil {
					return fmt.Errorf("reconcile derived deps for %s: %w", m.Name, rerr)
				}
			}
		}

		return printDepGraph(db, warnings)
	},
}

// depEdge is one model_depends_on edge with its provenance for display.
type depEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Source string `json:"source"` // "declared" | "derived" | raw source
}

// printDepGraph reads the current model_depends_on facts and prints them grouped
// by model, annotated declared/derived.
func printDepGraph(db *database.CatalogDB, warnings []string) error {
	facts, err := db.QueryFacts(database.FactFilter{Relation: modelDependsRelation})
	if err != nil {
		return err
	}
	var edges []depEdge
	for _, f := range facts {
		from, to := edgeArgs(f.Args)
		if from == "" || to == "" {
			continue
		}
		edges = append(edges, depEdge{From: from, To: to, Source: sourceLabel(f.Source)})
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
		fmt.Println(string(data))
		return nil
	}

	for _, w := range warnings {
		fmt.Printf("warning: %s\n", w)
	}
	if len(edges) == 0 {
		fmt.Println("No cross-model dependencies recorded.")
		return nil
	}
	fmt.Printf("Cross-model dependencies (%d edge(s)):\n\n", len(edges))
	var lastFrom string
	for _, e := range edges {
		if e.From != lastFrom {
			fmt.Printf("  %s depends on:\n", e.From)
			lastFrom = e.From
		}
		fmt.Printf("    → %s  [%s]\n", e.To, e.Source)
	}
	fmt.Println("\nQuery: pudl query depends_transitive from=<model> | impacted_by changed=<model> | --topo model_depends_on")
	return nil
}

// sourceLabel maps a fact source to a short provenance label.
func sourceLabel(source string) string {
	switch {
	case len(source) >= 6 && source[:6] == "model:":
		return "declared"
	case len(source) >= 8 && source[:8] == "derived:":
		return "derived"
	default:
		return source
	}
}

func init() {
	modelCmd.AddCommand(modelDepsCmd)
	modelDepsCmd.Flags().BoolVar(&modelDepsDerive, "derive", false, "also compute Phase-2 derived edges (desired↔produced identity matching)")
}
