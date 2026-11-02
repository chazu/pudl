package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/graphexport"
	"github.com/chazu/pudl/internal/importer"
)

var graphExportOutput string

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Export the resource graph",
	Args:  cobra.NoArgs,
	Long: `Work with the resource graph: cataloged resources and the relationships
between them, as read by the web graph explorer (web/).

Available subcommands:
- export:  Write the current resource graph as JSON`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var graphExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write the current resource graph as JSON",
	Args:  cobra.NoArgs,
	Long: `Write the current resource graph as JSON, in the canonical graph format the
web explorer loads (docs/frontend.md#backend-integration).

Resources are the catalog's current entries, one per resource. Each schema's
optional _pudl.graph block names the fields used for name, account, region and
zone, and the values other records use to refer to the resource.

Relationships come from Datalog rules whose head is the reserved relation

    graph_edge(type, source, target, basis)

where source and target are a resource ID or a reference value such as an
AWS ID or a GCP path. A reference that matches no cataloged resource is kept
as "ref:<value>", so the graph shows the resource as unknown instead of
dropping the edge. Problems (unreadable payloads, ambiguous references, rules
that fail) are listed in meta.warnings and never stop the export.

Output is always JSON; the global --json flag changes nothing.

Examples:
    pudl graph export > graph.json
    pudl graph export -o web/public/graph.json`,
	RunE: pudlRunE(runGraphExport),
}

func init() {
	rootCmd.AddCommand(graphCmd)
	graphCmd.AddCommand(graphExportCmd)
	graphExportCmd.Flags().StringVarP(&graphExportOutput, "output", "o", "", "Output file (default: stdout)")
}

func runGraphExport(cmd *cobra.Command, args []string) error {
	graph, err := buildWorkspaceGraph(cmd.Context())
	if err != nil {
		return err
	}

	var w io.Writer = outw()
	if graphExportOutput != "" {
		f, err := os.Create(graphExportOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(graph); err != nil {
		return err
	}
	if graphExportOutput != "" {
		fmt.Fprintf(errw(), "Wrote %d resources and %d relationships to %s (%d warnings)\n",
			len(graph.Resources), len(graph.Relationships), graphExportOutput, len(graph.Meta.Warnings))
	}
	return nil
}

// buildWorkspaceGraph exports the current workspace's resource graph. It
// opens the catalog and loads rules on every call, so a long-running caller
// (pudl ui) sees new observations and rules without a restart.
func buildWorkspaceGraph(ctx context.Context) (*graphexport.Graph, error) {
	configDir := effectivePudlDir()
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open catalog: %w", err)
	}
	defer db.Close()

	rules, err := datalog.LoadRulesFromPaths(queryRulePaths(configDir)...)
	if err != nil {
		return nil, fmt.Errorf("failed to load rules: %w", err)
	}
	reg, err := projectionRegistry()
	if err != nil {
		return nil, err
	}
	return graphexport.Build(ctx, graphexport.Input{
		DB:        db,
		Rules:     rules,
		Metadata:  reg.Source().GetSchemaMetadata,
		Load:      importer.DecodeEntry,
		Workspace: effectiveWorkspaceName(),
		Now:       time.Now(),
	})
}
