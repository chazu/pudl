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
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inventory"
)

// perInputModel enumerates hosts from hosts.json and cats <host>.json for
// each, one observation per host scoped test/<host>/things. Its check reads
// the run's own observations.
const perInputModel = `package systemmodel

#PerInput: #SystemModel & {
	name: "perinput"
	observation: {scope: "test/{{item}}/things", complete: true}
	populate: {
		each: argv: ["cat", %[1]q]
		runs: [{argv: ["cat", %[2]q], set: host: "{{item}}"}]
	}
	checks: [
		{name: "current", query: "perinput_failed", expect: "empty", severity: "fail", message: "failed", evidence: ["current"]},
	]
}
`

const perInputRules = `package rules

perinput_failed: {
	head: {rel: "perinput_failed", args: {target: "$T"}}
	body: [{rel: "catalog_entry", args: {target: "$T", status: "failed"}}]
}
`

// thingPolicy gives every thing a policy result, so a thing marked gone has
// its result closed.
const thingPolicy = `package resources

#ThingPolicy: {
	_pudl: {schema_type: "policy", base_schema: "pudl/resources.#Thing"}
	value: "ok" @pudl(rule="value-ok", severity="high")
}
`

// perInputFixture is a workspace with the per-input model, its rules and
// the thing policy.
type perInputFixture struct {
	t        *testing.T
	h        *checkHarness
	pudlDir  string
	fixtures string
}

func setupPerInput(t *testing.T) *perInputFixture {
	t.Helper()
	h, pudlDir := setupCheckFixture(t)
	fixtures := t.TempDir()
	model := fmt.Sprintf(perInputModel, filepath.Join(fixtures, "hosts.json"), filepath.Join(fixtures, "{{item}}.json"))
	schema := filepath.Join(pudlDir, "schema", "pudl")
	require.NoError(t, os.WriteFile(filepath.Join(schema, "systemmodel", "perinput.cue"), []byte(model), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(schema, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schema, "rules", "perinput.cue"), []byte(perInputRules), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(schema, "resources", "policy.cue"), []byte(thingPolicy), 0o644))
	return &perInputFixture{t: t, h: h, pudlDir: pudlDir, fixtures: fixtures}
}

// observe sets the hosts list and each host's things, removes the files of
// hosts given as nil, and runs the model.
func (f *perInputFixture) observe(hosts []string, things map[string][]string) (checkreport.ModelReport, error) {
	f.t.Helper()
	write := func(name, content string) {
		require.NoError(f.t, os.WriteFile(filepath.Join(f.fixtures, name), []byte(content), 0o644))
	}
	list := ""
	for i, h := range hosts {
		if i > 0 {
			list += ","
		}
		list += fmt.Sprintf("%q", h)
	}
	write("hosts.json", "["+list+"]")
	for host, names := range things {
		path := filepath.Join(f.fixtures, host+".json")
		if names == nil {
			_ = os.Remove(path)
			continue
		}
		records := ""
		for i, n := range names {
			if i > 0 {
				records += ","
			}
			records += fmt.Sprintf(`{"_schema":"resources.thing","name":%q,"value":"ok"}`, n)
		}
		write(host+".json", "["+records+"]")
	}
	err := f.h.run("perinput")
	report := latestCheckReportForTest(f.t, f.pudlDir)
	require.Len(f.t, report.Members, 1)
	return memberRunReportForTest(f.t, f.pudlDir, report.Members[0].RunID), err
}

func (f *perInputFixture) db() *database.CatalogDB {
	f.t.Helper()
	db, err := database.NewCatalogDB(f.pudlDir)
	require.NoError(f.t, err)
	f.t.Cleanup(func() { _ = db.Close() })
	return db
}

// policyResults counts the things with a current policy result.
func (f *perInputFixture) policyResults() int {
	f.t.Helper()
	db, err := database.NewCatalogDB(f.pudlDir)
	require.NoError(f.t, err)
	defer db.Close()
	facts, err := db.QueryFacts(database.FactFilter{Relation: database.PolicyStatusRelation})
	require.NoError(f.t, err)
	return len(facts)
}

// One failed host makes only its own observation incomplete: the other
// host's is complete, and the run's check, which reads both, is unknown.
func TestPerInputScopeFailedInputLeavesOnlyItsScopeIncomplete(t *testing.T) {
	f := setupPerInput(t)
	report, err := f.observe([]string{"A", "B"}, map[string][]string{"A": {"a1"}, "B": nil})
	require.ErrorIs(t, err, errFailedInputs)

	pr := report.Populate
	require.NotNil(t, pr)
	assert.Empty(t, pr.SnapshotID, "no single snapshot holds a per-input populate")
	assert.Equal(t, 1, pr.Records)
	require.Len(t, pr.Observations, 2)
	a, b := pr.Observations[0], pr.Observations[1]
	assert.Equal(t, checkreport.InputObservation{Input: "A", Scope: "test/A/things", SnapshotID: a.SnapshotID, Records: 1, Complete: true}, a)
	assert.Equal(t, checkreport.InputObservation{Input: "B", Scope: "test/B/things", SnapshotID: b.SnapshotID, Records: 0, Complete: false}, b)

	db := f.db()
	for _, o := range pr.Observations {
		snapshot, err := db.GetObserveSnapshot(o.SnapshotID)
		require.NoError(t, err)
		require.NotNil(t, snapshot)
		assert.Equal(t, o.Input, snapshot.Input)
		assert.Equal(t, o.Scope, snapshot.Scope)
		assert.Equal(t, o.Complete, snapshot.Complete)
	}

	require.Len(t, report.Checks, 1)
	c := report.Checks[0]
	assert.Equal(t, "unknown", c.Outcome)
	require.Len(t, c.Evidence, 2)
	assert.Equal(t, "incomplete_observation", c.Diagnostics[0].Code)
	assert.Contains(t, report.Markdown(), "- observations (2, one per input):")
}

// A context switch: observing host B never marks host A's things gone.
// Re-observing A without one thing does.
func TestPerInputScopeContextSwitchKeepsTheOtherScope(t *testing.T) {
	f := setupPerInput(t)
	first, err := f.observe([]string{"A"}, map[string][]string{"A": {"a1", "a2"}})
	require.NoError(t, err)
	require.Len(t, first.Populate.Observations, 1)
	aBefore := first.Populate.Observations[0].SnapshotID
	assert.Equal(t, 2, f.policyResults())

	second, err := f.observe([]string{"B"}, map[string][]string{"B": {"b1"}})
	require.NoError(t, err)
	assert.Equal(t, 3, f.policyResults(), "observing B closes nothing of A")
	require.Len(t, second.Checks, 1)
	assert.Equal(t, "pass", second.Checks[0].Outcome)
	require.Len(t, second.Checks[0].Evidence, 1)
	assert.Equal(t, "test/B/things", second.Checks[0].Evidence[0].Scope, "the check covers the observed scope only")

	db := f.db()
	current, err := db.CurrentScopeSnapshot("test/A/things")
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Equal(t, aBefore, current.SnapshotID, "A's latest observation is still A's")
	observed, err := db.ModelObservations("perinput")
	require.NoError(t, err)
	require.Len(t, observed, 1)
	assert.Equal(t, "test/B/things", observed[0].Scope)
	require.NoError(t, db.Close())

	third, err := f.observe([]string{"A"}, map[string][]string{"A": {"a1"}})
	require.NoError(t, err)
	assert.Equal(t, 2, f.policyResults(), "a2 left A's scope; b1 stays")
	comparison, err := inventory.CompareSnapshots(context.Background(), f.db(), aBefore, third.Populate.Observations[0].SnapshotID, f.h.ws.InventoryPolicy)
	require.NoError(t, err)
	changes := map[string]string{}
	for _, c := range comparison.Changes {
		changes[c.Resource] = c.Change
	}
	assert.Equal(t, map[string]string{"pudl/resources.#Thing/a2": "removed"}, changes)
}

// A fixed scope is one observation, as before: no per-input observations and
// no input on the snapshot.
func TestFixedScopeRecordsOneObservation(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	setupFanout(t, pudlDir)
	require.NoError(t, h.run("fanout"))
	pr := fanoutReport(t, pudlDir).Populate
	require.NotNil(t, pr)
	assert.Empty(t, pr.Observations)
	snapshot := fanoutSnapshot(t, pudlDir, pr.SnapshotID)
	assert.Equal(t, "test/things", snapshot.Scope)
	assert.Empty(t, snapshot.Input)
}
