package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checksRulesDir writes a rules dir holding one run-scoped rule (its head
// exposes run_id, so the constraint has a binding point) and one catalog-wide
// rule (its head does not).
func checksRulesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	rules := filepath.Join(dir, "rules")
	require.NoError(t, os.MkdirAll(rules, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rules, "checks.cue"), []byte(`package rules

// Head exposes run_id, so `+"`pudl run`"+` may bind this run's ID to it.
failed_this_run: {
	head: {rel: "failed_this_run", args: {run_id: "$R", target: "$T"}}
	body: [{rel: "catalog_entry", args: {run_id: "$R", target: "$T", status: "failed"}}]
}

// No run_id in the head: evaluates catalog-wide, as every rule does today.
failed_anywhere: {
	head: {rel: "failed_anywhere", args: {target: "$T"}}
	body: [{rel: "catalog_entry", args: {target: "$T", status: "failed"}}]
}
`), 0o644))
	return dir
}

// seedFailedEntry records one catalog entry in `failed` status under a run.
func seedFailedEntry(t *testing.T, db *database.CatalogDB, id, target, runID string) {
	t.Helper()
	status := "failed"
	entryType := "observe"
	require.NoError(t, db.AddEntry(database.CatalogEntry{
		ID:              id,
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test",
		Schema:          "test.#Resource",
		EntryType:       &entryType,
		Target:          &target,
		RunID:           &runID,
		Status:          &status,
	}))
}

func checksModel(query, expect string) *systemmodel.SystemModel {
	return &systemmodel.SystemModel{
		Name: "m",
		Checks: []systemmodel.Check{{
			Name:     "no-failures",
			Query:    query,
			Expect:   expect,
			Severity: "fail",
			Message:  "a resource is in failed status",
		}},
	}
}

func TestRunChecks_RunIDConstraintScopesToThisRun(t *testing.T) {
	dir := t.TempDir()
	cat := newRunCatalog(dir)
	t.Cleanup(func() { _ = cat.Close() })
	db, err := cat.required()
	require.NoError(t, err)

	// A failure belonging to an *earlier* run. The current run recorded nothing.
	seedFailedEntry(t, db, "e1", "old-resource", "run_previous")

	modelDir := checksRulesDir(t)
	unscopedRun := acute.NewTupleScope(nil, nil)

	// The run-scoped rule sees only this run's rows, so the earlier failure does
	// not gate it.
	scoped, err := runChecks(cat, checksModel("failed_this_run", "empty"), modelDir, checkContext{
		runID: "run_current",
		scope: unscopedRun,
	})
	require.NoError(t, err)
	require.Len(t, scoped, 1)
	assert.Equal(t, checkScopeRun, scoped[0].Scope)
	assert.Equal(t, 0, scoped[0].Count)
	assert.True(t, scoped[0].Passed)

	// The catalog-wide rule sees it, and is reported as global so the difference
	// is legible rather than silent.
	global, err := runChecks(cat, checksModel("failed_anywhere", "empty"), modelDir, checkContext{
		runID: "run_current",
		scope: unscopedRun,
	})
	require.NoError(t, err)
	require.Len(t, global, 1)
	assert.Equal(t, checkScopeGlobal, global[0].Scope)
	assert.Equal(t, 1, global[0].Count)
	assert.False(t, global[0].Passed)
}

func TestRunChecks_RunIDConstraintSeesThisRunsOwnRows(t *testing.T) {
	// The complement of the test above: scoping must not make every `empty` check
	// pass trivially. A failure recorded *by this run* still gates.
	dir := t.TempDir()
	cat := newRunCatalog(dir)
	t.Cleanup(func() { _ = cat.Close() })
	db, err := cat.required()
	require.NoError(t, err)

	seedFailedEntry(t, db, "e1", "current-resource", "run_current")

	results, err := runChecks(cat, checksModel("failed_this_run", "empty"), checksRulesDir(t), checkContext{
		runID: "run_current",
		scope: acute.NewTupleScope(nil, nil),
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkScopeRun, results[0].Scope)
	assert.Equal(t, 1, results[0].Count)
	assert.False(t, results[0].Passed)
}

func TestRunChecks_ReplayNeverBindsRunID(t *testing.T) {
	// A replay observed nothing, so no row carries its run ID. Binding it would
	// make every `expect: empty` check pass trivially — a false pass produced by
	// the fix meant to prevent false passes.
	dir := t.TempDir()
	cat := newRunCatalog(dir)
	t.Cleanup(func() { _ = cat.Close() })
	db, err := cat.required()
	require.NoError(t, err)

	seedFailedEntry(t, db, "e1", "old-resource", "run_previous")

	results, err := runChecks(cat, checksModel("failed_this_run", "empty"), checksRulesDir(t), checkContext{
		runID:       "run_replay",
		fromCatalog: true,
		scope:       acute.NewTupleScope(nil, nil),
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkScopeGlobal, results[0].Scope, "a replay reports its checks as unscoped")
	assert.Equal(t, 1, results[0].Count, "the earlier failure is still seen")
	assert.False(t, results[0].Passed)
}

func TestRunChecks_OnlyPartitionsResultTuples(t *testing.T) {
	dir := t.TempDir()
	cat := newRunCatalog(dir)
	t.Cleanup(func() { _ = cat.Close() })
	db, err := cat.required()
	require.NoError(t, err)

	seedFailedEntry(t, db, "e1", "db", "run_current")

	model := &systemmodel.SystemModel{
		Name: "m",
		Desired: []map[string]any{
			{"name": "app", "kind": "Deployment"},
			{"name": "db", "kind": "StatefulSet"},
		},
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
		Checks: []systemmodel.Check{{
			Name: "no-failures", Query: "failed_anywhere", Expect: "empty",
			Severity: "fail", Message: "a resource is in failed status",
		}},
	}
	effective, err := acute.ScopeModelForRun(model, []string{"app"})
	require.NoError(t, err)

	results, err := runChecks(cat, effective, checksRulesDir(t), checkContext{
		runID: "run_current",
		scope: acute.NewTupleScope(model, effective),
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].Count, "the failure is on the excluded resource")
	assert.Equal(t, 1, results[0].AdvisoryCount)
	assert.True(t, results[0].Passed, "an out-of-scope failure does not gate a scoped run")
	assert.False(t, anyFailSeverityFailed(results))
}

func TestRunChecks_OnlyStillGatesOnInScopeFailure(t *testing.T) {
	dir := t.TempDir()
	cat := newRunCatalog(dir)
	t.Cleanup(func() { _ = cat.Close() })
	db, err := cat.required()
	require.NoError(t, err)

	seedFailedEntry(t, db, "e1", "app", "run_current")
	seedFailedEntry(t, db, "e2", "db", "run_current")

	model := &systemmodel.SystemModel{
		Name: "m",
		Desired: []map[string]any{
			{"name": "app", "kind": "Deployment"},
			{"name": "db", "kind": "StatefulSet"},
		},
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
		Checks: []systemmodel.Check{{
			Name: "no-failures", Query: "failed_anywhere", Expect: "empty",
			Severity: "fail", Message: "a resource is in failed status",
		}},
	}
	effective, err := acute.ScopeModelForRun(model, []string{"app"})
	require.NoError(t, err)

	results, err := runChecks(cat, effective, checksRulesDir(t), checkContext{
		runID: "run_current",
		scope: acute.NewTupleScope(model, effective),
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 1, results[0].Count)
	assert.Equal(t, 1, results[0].AdvisoryCount)
	assert.False(t, results[0].Passed)
	assert.True(t, anyFailSeverityFailed(results))
}
