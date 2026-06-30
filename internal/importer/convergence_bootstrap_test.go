package importer

import (
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/datalog"
)

// The cross-model convergence rules must ship in the bootstrap tree, parse as
// non-definition rules, and be picked up by the rule loader. This guards both
// the convergence.cue syntax and that CopyBootstrapSchemas installs it.
func TestConvergenceRulesShipAndParse(t *testing.T) {
	dir := t.TempDir()
	if err := CopyBootstrapSchemas(dir); err != nil {
		t.Fatalf("CopyBootstrapSchemas: %v", err)
	}

	rulesDir := filepath.Join(dir, "pudl", "rules")
	rules, err := datalog.LoadRulesFromPaths(rulesDir)
	if err != nil {
		t.Fatalf("LoadRulesFromPaths: %v", err)
	}

	want := map[string]bool{
		"depends_transitive": false,
		"impacted_by":        false,
		"cyclic":             false,
	}
	for _, r := range rules {
		if _, ok := want[r.Head.Rel]; ok {
			want[r.Head.Rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("shipped rules missing head relation %q", rel)
		}
	}
}
