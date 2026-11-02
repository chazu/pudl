package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

func checksOnlyWorkspace(t *testing.T) string {
	t.Helper()
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "gcp", "firewall.cue"), fmt.Sprintf(e2eFirewallBase, e2eFirewallFacts))
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "rules", "gcp.cue"), strings.Split(e2eRules, "typo_rule")[0])
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp.cue"), e2eCommandModels)
	r := runCLI(t, "check", "gcp-posture", "--json")
	require.Equal(t, "checks-only", checkedModel(t, r.Stdout).Mode, r.Stdout+r.Stderr)
	return pudlDir
}

// requireModelInstancesOK runs `pudl doctor --json` and requires every stored
// #SystemModel instance to validate, returning how many there are.
func requireModelInstancesOK(t *testing.T) int {
	t.Helper()
	r := runCLI(t, "doctor", "--json")
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout+r.Stderr)
	instances := 0
	for _, entry := range report.Catalog {
		if entry.Schema == systemmodel.SchemaName {
			instances++
			require.Equal(t, "ok", entry.Status, entry.Error)
		}
	}
	require.True(t, report.OK, r.Stdout)
	require.NoError(t, r.Err)
	return instances
}

// A checks-only model's stored instance must satisfy #SystemModel, so
// `pudl doctor` reports it valid (pudl-qhx).
func TestChecksOnlyModelInstancePassesDoctor(t *testing.T) {
	checksOnlyWorkspace(t)
	require.Equal(t, 1, requireModelInstancesOK(t))
}

// Catalogs written before the fix hold the instance with `populate: {}`.
// Doctor validates that exact legacy shape without the empty key.
func TestLegacyChecksOnlyModelInstancePassesDoctor(t *testing.T) {
	pudlDir := checksOnlyWorkspace(t)
	legacy := map[string]any{"name": "gcp-posture", "populate": map[string]any{}, "_schema": "systemmodel.system_model",
		"checks": []any{map[string]any{"name": "no-open-ingress", "query": "open_ingress", "expect": "empty", "severity": "fail", "message": "firewall open to the internet"}}}
	wrapped, err := json.Marshal([]any{map[string]any{"target": modelTarget("gcp-posture"), "current": map[string]any{"records": []any{legacy}}}})
	require.NoError(t, err)
	cat := newRunCatalog(pudlDir)
	_, _, err = ingestPopulateOutput(cat, wrapped, populateIngest{runID: "legacy", model: "gcp-posture", source: database.SnapshotSourceModelInstance})
	require.NoError(t, err)
	require.NoError(t, cat.Close())

	require.Equal(t, 2, requireModelInstancesOK(t), "the fixed and the legacy instance")
}
