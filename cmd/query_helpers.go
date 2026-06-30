package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
)

// loadQueryRules loads the workspace Datalog rules (global ~/.pudl rules, then
// repo-scoped rules which shadow them) plus any ad-hoc -f file. Shared by the
// query, --list, and --topo paths.
func loadQueryRules(configDir string) ([]datalog.Rule, error) {
	rulePaths := []string{filepath.Join(configDir, "schema", "pudl", "rules")}
	if wsCtx != nil && wsCtx.Workspace != nil {
		rulePaths = append(rulePaths, filepath.Join(wsCtx.Workspace.PudlDir, "schema", "pudl", "rules"))
	}
	rules, err := datalog.LoadRulesFromPaths(rulePaths...)
	if err != nil {
		return nil, fmt.Errorf("failed to load rules: %w", err)
	}
	if queryRuleFile != "" {
		fileRules, err := loadRulesFromFile(queryRuleFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load rules from %s: %w", queryRuleFile, err)
		}
		rules = append(rules, fileRules...)
	}
	return rules, nil
}

// runQueryList prints the relations a user/agent can query — derived rule heads
// (with the arg keys each expects) and stored EDB fact relations — closing the
// discoverability gap where rule-head relations never appear in shell completion
// (which lists only fact-table relations).
func runQueryList() error {
	configDir := config.GetPudlDir()

	rules, err := loadQueryRules(configDir)
	if err != nil {
		return err
	}

	// Derived relations: head rel -> union of head arg keys.
	derived := map[string]map[string]struct{}{}
	for _, r := range rules {
		keys := derived[r.Head.Rel]
		if keys == nil {
			keys = map[string]struct{}{}
			derived[r.Head.Rel] = keys
		}
		for k := range r.Head.Args {
			keys[k] = struct{}{}
		}
	}

	fmt.Println("Derived relations (from Datalog rules — query by constraining the arg keys):")
	if len(derived) == 0 {
		fmt.Println("  (none — no rules loaded)")
	}
	for _, rel := range sortedStringKeys(derived) {
		fmt.Printf("  %s(%s)\n", rel, strings.Join(sortedSetKeys(derived[rel]), ", "))
	}

	// EDB fact relations actually present in the store.
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		return fmt.Errorf("failed to open catalog: %w", err)
	}
	defer db.Close()

	rels, err := db.GetDistinctRelations()
	if err != nil {
		return fmt.Errorf("failed to list fact relations: %w", err)
	}
	fmt.Println("\nEDB fact relations (stored facts you can query or join against):")
	if len(rels) == 0 {
		fmt.Println("  (none recorded yet)")
	}
	for _, r := range rels {
		fmt.Printf("  %s\n", r)
	}
	fmt.Println("\nBuilt-in (join-only): catalog_entry — usable as a rule body atom, not queried directly.")
	return nil
}

// printTopoOrder reads the result tuples as from/to dependency edges and prints a
// topological run order (dependencies first). model_depends_on(from,to) means
// `from` depends on `to`, so `to` is ordered before `from`. Errors on a cycle.
func printTopoOrder(relation string, results []datalog.Tuple) error {
	deps := map[string]map[string]struct{}{}       // node -> set it depends on
	dependents := map[string]map[string]struct{}{} // node -> set that depends on it
	nodes := map[string]struct{}{}

	ensure := func(m map[string]map[string]struct{}, k string) map[string]struct{} {
		if m[k] == nil {
			m[k] = map[string]struct{}{}
		}
		return m[k]
	}

	for _, t := range results {
		from, fok := t.Args["from"].(string)
		to, tok := t.Args["to"].(string)
		if !fok || !tok {
			return fmt.Errorf("--topo needs from/to edges; relation %q tuple has args %v (try model_depends_on or depends_transitive)", relation, t.Args)
		}
		nodes[from] = struct{}{}
		nodes[to] = struct{}{}
		ensure(deps, from)[to] = struct{}{}
		ensure(dependents, to)[from] = struct{}{}
	}

	if len(nodes) == 0 {
		fmt.Println("No edges; nothing to order.")
		return nil
	}

	indeg := map[string]int{}
	for n := range nodes {
		indeg[n] = len(deps[n])
	}

	var order []string
	ready := readyNodes(indeg, 0)
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		order = append(order, n)
		delete(indeg, n)
		for d := range dependents[n] {
			indeg[d]--
			if indeg[d] == 0 {
				ready = insertSorted(ready, d)
			}
		}
	}

	if len(order) != len(nodes) {
		return fmt.Errorf("dependency cycle detected among %d model(s); no valid run order — run `pudl query cyclic` to see the models involved", len(nodes)-len(order))
	}

	for i, n := range order {
		fmt.Printf("%d. %s\n", i+1, n)
	}
	return nil
}

// readyNodes returns the sorted nodes whose current indegree equals want.
func readyNodes(indeg map[string]int, want int) []string {
	var out []string
	for n, d := range indeg {
		if d == want {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// insertSorted inserts s into the already-sorted slice, preserving order.
func insertSorted(slice []string, s string) []string {
	i := sort.SearchStrings(slice, s)
	slice = append(slice, "")
	copy(slice[i+1:], slice[i:])
	slice[i] = s
	return slice
}

func sortedStringKeys(m map[string]map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSetKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
