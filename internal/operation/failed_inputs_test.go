package operation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/checks"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inventory"
)

// fanoutModel enumerates projects from projects.json and cats <project>.json
// for each, so deleting a project's file makes its run fail. Its checks read
// the catalog three ways: latest-known, scoped to the run, and through the
// run's own snapshot.
const fanoutModel = `package systemmodel

#Fanout: #SystemModel & {
	name: "fanout"
	observation: {scope: "test/things", complete: true}
	populate: {
		each: argv: ["cat", %[1]q]
		runs: [{argv: ["cat", %[2]q], set: project: "{{item}}"}]
	}
	checks: [
		{name: "latest", query: "failed_anywhere", expect: "empty", severity: "fail", message: "failed"},
		{name: "this-run", query: "failed_this_run", expect: "empty", severity: "fail", message: "failed"},
		{name: "current", query: "failed_anywhere", expect: "empty", severity: "fail", message: "failed", evidence: ["current"]},
	]
}
`

const fanoutRules = `package rules

failed_this_run: {
	head: {rel: "failed_this_run", args: {run_id: "$R", target: "$T"}}
	body: [{rel: "catalog_entry", args: {run_id: "$R", target: "$T", status: "failed"}}]
}

failed_anywhere: {
	head: {rel: "failed_anywhere", args: {target: "$T"}}
	body: [{rel: "catalog_entry", args: {target: "$T", status: "failed"}}]
}
`

// setupFanout writes the fan-out model, its rules and three project fixtures,
// and returns the fixture directory.
func setupFanout(t *testing.T, pudlDir string) string {
	t.Helper()
	fixtures := t.TempDir()
	files := map[string]string{
		"projects.json": `["p1", "p2", "p3"]`,
		"p1.json":       `[{"_schema":"resources.thing","name":"a1","value":"ok"}]`,
		"p2.json":       `[{"_schema":"resources.thing","name":"b1","value":"ok"}]`,
		"p3.json":       `[{"_schema":"resources.thing","name":"c1","value":"ok"}]`,
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(fixtures, name), []byte(content), 0o644))
	}
	model := fmt.Sprintf(fanoutModel, filepath.Join(fixtures, "projects.json"), filepath.Join(fixtures, "{{item}}.json"))
	schema := filepath.Join(pudlDir, "schema", "pudl")
	require.NoError(t, os.WriteFile(filepath.Join(schema, "systemmodel", "fanout.cue"), []byte(model), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(schema, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schema, "rules", "fanout.cue"), []byte(fanoutRules), 0o644))
	return fixtures
}

func fanoutReport(t *testing.T, pudlDir string) checkreport.ModelReport {
	t.Helper()
	report := latestCheckReportForTest(t, pudlDir)
	require.Len(t, report.Members, 1)
	return memberRunReportForTest(t, pudlDir, report.Members[0].RunID)
}

func fanoutSnapshot(t *testing.T, pudlDir, id string) *database.ObserveSnapshot {
	t.Helper()
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	snapshot, err := db.GetObserveSnapshot(id)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	return snapshot
}

// With every input observed, the fan-out behaves as a static one: one
// complete snapshot, passing checks, no failed inputs.
func TestCheckFanoutWithoutFailuresIsComplete(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	setupFanout(t, pudlDir)

	require.NoError(t, h.run("fanout"))
	report := fanoutReport(t, pudlDir)
	assert.Equal(t, database.RunStatusSucceeded, report.CompletionStatus)
	require.NotNil(t, report.Populate)
	assert.Equal(t, 3, report.Populate.Records)
	assert.Equal(t, []string{"p1", "p2", "p3"}, report.Populate.Inputs)
	assert.Empty(t, report.Populate.FailedInputs)
	for _, c := range report.Checks {
		assert.Equal(t, "pass", c.Outcome, c.Name)
	}
	assert.True(t, fanoutSnapshot(t, pudlDir, report.Populate.SnapshotID).Complete)
}

// One failing project: the others are ingested, the snapshot is incomplete,
// every check is unknown rather than passing, and the report names the input.
func TestCheckFanoutIngestsTheRestWhenOneInputFails(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	fixtures := setupFanout(t, pudlDir)
	require.NoError(t, h.run("fanout"))
	before := fanoutReport(t, pudlDir).Populate.SnapshotID

	require.NoError(t, os.Remove(filepath.Join(fixtures, "p2.json")))
	err := h.run("fanout")
	require.ErrorIs(t, err, errFailedInputs)
	require.ErrorContains(t, err, "1 of 3 runs failed (p2)")

	report := fanoutReport(t, pudlDir)
	assert.Equal(t, database.RunStatusFailed, report.CompletionStatus)
	pr := report.Populate
	require.NotNil(t, pr)
	assert.Equal(t, 0, pr.Records, "p1 and p3 are unchanged since the first run")
	assert.Equal(t, 3, pr.Runs)
	require.Len(t, pr.FailedInputs, 1)
	failed := pr.FailedInputs[0]
	assert.Equal(t, "p2", failed.Item)
	assert.Equal(t, 1, failed.Run)
	assert.Equal(t, 1, failed.ExitStatus)
	assert.Equal(t, []string{"cat", filepath.Join(fixtures, "p2.json")}, failed.Argv)
	assert.Contains(t, failed.Stderr, "p2.json")

	snapshot := fanoutSnapshot(t, pudlDir, pr.SnapshotID)
	assert.False(t, snapshot.Complete, "a failed input leaves the observation incomplete")
	assert.Equal(t, "test/things", snapshot.Scope)
	assert.Equal(t, 2, snapshot.RecordCount, "p1 and p3 are in the snapshot")

	outcomes := map[string]checkreport.CheckResult{}
	for _, c := range report.Checks {
		outcomes[c.Name] = c
	}
	require.Len(t, outcomes, 3)
	for name, scope := range map[string]string{"latest": checks.ScopeLatestKnown, "this-run": checks.ScopeRun, "current": checks.ScopeSnapshots} {
		c := outcomes[name]
		assert.Equal(t, "unknown", c.Outcome, name)
		assert.False(t, c.Passed, name)
		assert.Equal(t, scope, c.Scope, name)
		require.NotEmpty(t, c.Diagnostics, name)
	}
	assert.Equal(t, "failed_inputs", outcomes["latest"].Diagnostics[0].Code)
	assert.Equal(t, "failed_inputs", outcomes["this-run"].Diagnostics[0].Code)
	assert.Equal(t, "incomplete_observation", outcomes["current"].Diagnostics[0].Code)

	// p2's resource is not observed, not removed: the scope stays open for it.
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	comparison, err := inventory.CompareSnapshots(context.Background(), db, before, pr.SnapshotID, h.ws.InventoryPolicy)
	require.NoError(t, err)
	changes := map[string]string{}
	for _, c := range comparison.Changes {
		changes[c.Resource] = c.Change
	}
	assert.Equal(t, "not-observed", changes["pudl/resources.#Thing/b1"], "changes: %v", changes)

	text := report.Markdown()
	assert.Contains(t, text, "- inputs (3): p1, p2, p3")
	assert.Contains(t, text, "- failed inputs: 1 of 3 runs")
	assert.Contains(t, text, "  - p2: exit 1: exit status 1")
}

// When every input fails nothing is ingested, as before, and the report still
// lists the failures.
func TestCheckFanoutIngestsNothingWhenEveryInputFails(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	fixtures := setupFanout(t, pudlDir)
	for _, p := range []string{"p1", "p2", "p3"} {
		require.NoError(t, os.Remove(filepath.Join(fixtures, p+".json")))
	}
	err := h.run("fanout")
	require.ErrorContains(t, err, "populate: all 3 runs failed, so nothing was ingested")

	report := fanoutReport(t, pudlDir)
	require.NotNil(t, report.Populate)
	assert.Len(t, report.Populate.FailedInputs, 3)
	assert.Empty(t, report.Populate.SnapshotID)
}
