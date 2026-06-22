package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/systemmodel"
)

// CheckResult is the outcome of one model check (a Datalog relation evaluated
// over the catalog, asserted empty/nonempty).
type CheckResult struct {
	Name     string
	Query    string
	Severity string
	Count    int
	Passed   bool
	Message  string
}

// checkPasses is the pure expect-vs-count verdict: "empty" passes on no tuples,
// "nonempty" passes on at least one.
func checkPasses(expect string, count int) bool {
	switch expect {
	case "empty":
		return count == 0
	case "nonempty":
		return count > 0
	default:
		return false
	}
}

// ruleSearchPaths returns the existing directories to load Datalog rules from:
// the global pudl rules, the repo-scoped rules (when in a workspace), and the
// model's own rules/ subdir. Missing dirs are skipped (the loader errors on them).
func ruleSearchPaths(modelDir string) []string {
	candidates := []string{
		filepath.Join(config.GetPudlDir(), "schema", "pudl", "rules"),
		filepath.Join(modelDir, "rules"),
	}
	if wsCtx != nil && wsCtx.Workspace != nil {
		candidates = append(candidates,
			filepath.Join(wsCtx.Workspace.PudlDir, "schema", "pudl", "rules"))
	}
	var paths []string
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			paths = append(paths, p)
		}
	}
	return paths
}

// runChecks evaluates each of the model's checks (a Datalog relation over the
// catalog) and returns the per-check verdicts. Rules are loaded from the standard
// pudl paths plus the model's rules/ subdir.
func runChecks(m *systemmodel.SystemModel, modelDir string) ([]CheckResult, error) {
	if len(m.Checks) == 0 {
		return nil, nil
	}
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	defer db.Close()

	rules, err := datalog.LoadRulesFromPaths(ruleSearchPaths(modelDir)...)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}

	var results []CheckResult
	for _, c := range m.Checks {
		tuples, err := datalog.Evaluate(db, rules, c.Query, nil, datalog.TemporalScope{})
		if err != nil {
			return nil, fmt.Errorf("check %q (relation %q): %w", c.Name, c.Query, err)
		}
		results = append(results, CheckResult{
			Name:     c.Name,
			Query:    c.Query,
			Severity: c.Severity,
			Count:    len(tuples),
			Passed:   checkPasses(c.Expect, len(tuples)),
			Message:  c.Message,
		})
	}
	return results, nil
}

// printChecks renders the check results and reports whether any check with
// severity "fail" did not pass (the caller turns that into a non-zero exit).
func printChecks(results []CheckResult) (failedFail bool) {
	for _, r := range results {
		mark := "ok"
		if !r.Passed {
			mark = "FAIL"
			if r.Severity == "fail" {
				failedFail = true
			}
		}
		if r.Passed {
			fmt.Printf("  ✓ %s (%s)\n", r.Name, r.Severity)
		} else {
			fmt.Printf("  ✗ %s [%s] %s — %d match(es): %s\n", r.Name, r.Severity, mark, r.Count, r.Message)
		}
	}
	return failedFail
}
