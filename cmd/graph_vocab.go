package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/graphexport"
	"github.com/chazu/pudl/internal/graphvocab"
)

var graphVocabCmd = &cobra.Command{
	Use:   "vocab",
	Short: "Show what pudl ui knows about resource and relationship types",
	Args:  cobra.NoArgs,
	Long: `Show the graph vocabulary: everything pudl ui knows about types, read from
CUE. pudl ui has no type knowledge of its own; a type or relationship missing
here is drawn as unknown.

- Resource types: each schema's _pudl graph block (display {code, label,
  class} and view roles), plus graph_resource_type facts for types with no
  schema.
- Edge types and categories: graph_edge_type and graph_edge_category facts
  in rule files (pudl/rules/*.cue).
- Providers: graph_provider facts; graph_account_type names the type a
  provider's account attribute refers to.

Problems lists what will not draw as intended: cataloged types with no display
info, graph_edge rules deriving undeclared types, roles naming undeclared edge
types, unknown categories and conflicting declarations. pudl doctor reports
the same problems.

--json prints exactly the vocabulary field of pudl graph export.

See docs/schema-authoring.md, "Make a type appear in pudl ui".`,
	Example: `  pudl graph vocab
  pudl graph vocab --json | jq '.problems'`,
	RunE: pudlRunE(func(cmd *cobra.Command, args []string) error {
		v, err := workspaceVocabulary(effectivePudlDir())
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(v)
		}
		printVocabulary(outw(), v)
		return nil
	}),
}

func init() {
	graphCmd.AddCommand(graphVocabCmd)
}

// workspaceVocabulary builds the graph vocabulary from the workspace's
// schemas and rules, and notes cataloged types with no display info.
func workspaceVocabulary(configDir string) (*graphvocab.Vocabulary, error) {
	rules, err := datalog.LoadRulesFromPaths(queryRulePaths(configDir)...)
	if err != nil {
		return nil, fmt.Errorf("failed to load rules: %w", err)
	}
	reg, err := projectionRegistry()
	if err != nil {
		return nil, err
	}
	src := reg.Source()
	v := graphexport.Vocabulary(src.GetAvailableSchemas(), src.GetSchemaMetadata, rules)

	// Diagnostics must not create a catalog: with none, nothing is cataloged.
	if _, err := os.Stat(filepath.Join(configDir, "data", "sqlite", "catalog.db")); os.IsNotExist(err) {
		return v, nil
	}
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open catalog: %w", err)
	}
	defer db.Close()
	counts, err := db.SchemaCoverageCounts()
	if err != nil {
		return nil, err
	}
	var types []string
	for _, c := range counts {
		m, ok := src.GetSchemaMetadata(c.Schema)
		if ok && m.ResourceType != "" && !graphexport.InternalTypes[m.ResourceType] && c.Entries > c.Unidentified {
			types = append(types, m.ResourceType)
		}
	}
	v.NoteMissingDisplay(types)
	return v, nil
}

func printVocabulary(w io.Writer, v *graphvocab.Vocabulary) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Resource types (%d)\n", len(v.ResourceTypes))
	fmt.Fprintln(tw, "  TYPE\tCODE\tLABEL\tCLASS\tROLES\tFROM")
	for _, rt := range v.ResourceTypes {
		from := rt.Schema
		if from == "" {
			from = rt.Source
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n", rt.Type, orDash(rt.Code), orDash(rt.Label), orDash(rt.Class), orDash(roleSummary(rt.Roles)), shortSource(from))
	}
	fmt.Fprintf(tw, "\nEdge types (%d)\n", len(v.EdgeTypes))
	fmt.Fprintln(tw, "  TYPE\tCATEGORY\tLABEL\tFROM")
	for _, e := range v.EdgeTypes {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", e.Type, e.Category, e.Label, shortSource(e.Source))
	}
	fmt.Fprintf(tw, "\nCategories (%d)\n", len(v.Categories))
	for _, c := range v.Categories {
		fmt.Fprintf(tw, "  %s\t%s\n", c.ID, c.Label)
	}
	fmt.Fprintf(tw, "\nProviders (%d)\n", len(v.Providers))
	for _, p := range v.Providers {
		fmt.Fprintf(tw, "  %s\t%s\n", p.Provider, p.Label)
	}
	_ = tw.Flush()
	if len(v.Problems) == 0 {
		fmt.Fprintln(w, "\nNo problems.")
		return
	}
	fmt.Fprintf(w, "\nProblems (%d)\n", len(v.Problems))
	for _, p := range v.Problems {
		fmt.Fprintf(w, "  - %s\n", p)
	}
}

// roleSummary renders a type's view roles compactly, e.g.
// "network(group, via in_vpc) security(control)".
func roleSummary(r graphvocab.Roles) string {
	var parts []string
	if r.Admin != nil {
		parts = append(parts, roleJoin("admin", roleFlag(r.Admin.Owner, "owner"), roleVia("parent", r.Admin.Parent)))
	}
	if r.Network != nil {
		parts = append(parts, roleJoin("network", roleFlag(r.Network.Group, "group"), roleVia("via", r.Network.Via), roleFlag(r.Network.Region, "region")))
	}
	if r.PortOf != "" {
		parts = append(parts, "port_of("+r.PortOf+")")
	}
	if r.Security != nil && r.Security.Control {
		parts = append(parts, "security(control)")
	}
	if r.Inventory != nil && r.Inventory.Collapsed {
		parts = append(parts, "inventory(collapsed)")
	}
	if r.Application != nil {
		parts = append(parts, roleJoin("application", roleFlag(r.Application.Group, "group"), roleVia("via", r.Application.Via)))
	}
	return strings.Join(parts, " ")
}

func roleFlag(on bool, name string) string {
	if on {
		return name
	}
	return ""
}

func roleVia(name string, types []string) string {
	if len(types) == 0 {
		return ""
	}
	return name + " " + strings.Join(types, "|")
}

// roleJoin renders name(part, part), or just name when no part is set.
func roleJoin(name string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return name
	}
	return name + "(" + strings.Join(out, ", ") + ")"
}

// shortSource trims a fact's source "name (/abs/path/file.cue:1:2)" to
// "file.cue:1:2"; --json keeps the full form.
func shortSource(s string) string {
	open := strings.LastIndex(s, " (")
	if open < 0 || !strings.HasSuffix(s, ")") {
		return s
	}
	return filepath.Base(s[open+2 : len(s)-1])
}
