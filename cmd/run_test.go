package cmd

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

func TestValidateRunFlags(t *testing.T) {
	cases := []struct {
		name    string
		f       runFlags
		wantErr string
	}{
		{"bare observe", runFlags{}, ""},
		{"converge alone", runFlags{converge: true, maxIters: 5}, ""},
		{"only without converge", runFlags{onlySet: true}, "--only requires --converge"},
		{"dry-run without converge", runFlags{dryRunSet: true}, "--dry-run requires --converge"},
		{"max-iters without converge", runFlags{maxItersSet: true}, "--max-iters requires --converge"},
		{"only with converge", runFlags{converge: true, onlySet: true, maxIters: 5}, ""},
		{"bad max-iters", runFlags{converge: true, maxIters: 0}, "--max-iters must be >= 1"},
		// An unscoped replay would set-diff desired against every observation in
		// the catalog, so it must be refused before any query runs.
		{"from-catalog without scope", runFlags{fromCatalog: true}, "--from-catalog requires --catalog-scope"},
		{"from-catalog with blank scope", runFlags{fromCatalog: true, catalogScope: "   "}, "--from-catalog requires --catalog-scope"},
		{"from-catalog with scope", runFlags{fromCatalog: true, catalogScope: "snap-1"}, ""},
		{"scope without from-catalog", runFlags{catalogScope: "snap-1"}, "--catalog-scope requires --from-catalog"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRunFlags(tc.f)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestBuildRunPlan_ObserveOnly(t *testing.T) {
	m := &systemmodel.SystemModel{
		Name:     "k8s-policy",
		Populate: systemmodel.Populate{Plugin: "k8s", Input: map[string]any{"namespace": "default"}},
		Checks:   []systemmodel.Check{{Name: "pdb"}},
	}
	plan := buildRunPlan(m, runFlags{})
	assert.Contains(t, plan, "model:    k8s-policy")
	assert.Contains(t, plan, "populate: observe (k8s)")
	assert.Contains(t, plan, "checks:   1")
	assert.Contains(t, plan, "mode:     observe-only")
}

func TestBuildRunPlan_ConvergeNoOpWhenNotConvergent(t *testing.T) {
	m := &systemmodel.SystemModel{
		Name:     "k8s-policy",
		Populate: systemmodel.Populate{Plugin: "k8s"},
	}
	// --converge on an observe-only model is a no-op, said so explicitly.
	plan := buildRunPlan(m, runFlags{converge: true, maxIters: 5})
	assert.Contains(t, plan, "no-op")
}

func TestBuildRunPlan_Converge(t *testing.T) {
	m := &systemmodel.SystemModel{
		Name:     "k8s-converge",
		Populate: systemmodel.Populate{Plugin: "k8s"},
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
		Desired:  []map[string]any{{"name": "web"}},
	}
	plan := buildRunPlan(m, runFlags{converge: true, maxIters: 3, dryRun: true, only: []string{"web"}})
	assert.Contains(t, plan, `converge via "k8s"`)
	assert.Contains(t, plan, "max-iters 3")
	assert.Contains(t, plan, "dry-run")
	assert.Contains(t, plan, "only: web")
	assert.True(t, strings.Contains(plan, "loop:"), "converge plan should show the loop phase")
}

func TestUseInventoryDrift(t *testing.T) {
	diff := &systemmodel.SystemModel{Populate: systemmodel.Populate{Plugin: "k8s", Differential: true}}
	inv := &systemmodel.SystemModel{Populate: systemmodel.Populate{Plugin: "host", Differential: false}}
	ewe := &systemmodel.SystemModel{Populate: systemmodel.Populate{EweSource: "populate.cue"}}

	assert.False(t, useInventoryDrift(diff, false), "differential observer -> differential drift")
	assert.True(t, useInventoryDrift(diff, true), "--from-catalog forces inventory")
	assert.True(t, useInventoryDrift(inv, false), "non-differential observer -> inventory")
	assert.True(t, useInventoryDrift(ewe, false), "ewe populate -> inventory")
}

func TestScopeModelForRun(t *testing.T) {
	model := &systemmodel.SystemModel{
		Name:     "example",
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
		Desired: []map[string]any{
			{"_schema": "pudl/k8s.#Deployment", "name": "web", "kind": "Deployment"},
			{"_schema": "pudl/k8s.#Service", "name": "api", "kind": "Service", "depends_on": []any{"web"}},
		},
	}

	scoped, err := scopeModelForRun(model, []string{"web", "Service"})
	require.NoError(t, err)
	require.Len(t, scoped.Desired, 2)

	scoped, err = scopeModelForRun(model, []string{"web"})
	require.NoError(t, err)
	require.Len(t, scoped.Desired, 1)
	assert.Equal(t, "web", scoped.Desired[0]["name"])

	scoped, err = scopeModelForRun(model, []string{"api"})
	require.NoError(t, err)
	require.Len(t, scoped.Desired, 2)

	_, err = scopeModelForRun(model, []string{"missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not match")
}

func TestRunVerdict(t *testing.T) {
	cases := []struct {
		name   string
		report *RunReport
		flags  runFlags
		want   string
	}{
		{"converge reaches clean", &RunReport{Converge: &ConvergeReport{Outcome: "clean"}}, runFlags{converge: true}, "clean"},
		{"converge cap failed", &RunReport{Converge: &ConvergeReport{Outcome: "failed (cap_exhausted)"}}, runFlags{converge: true}, "failed"},
		{"converge exec failed", &RunReport{Converge: &ConvergeReport{Outcome: "failed (execute_error)"}}, runFlags{converge: true}, "failed"},
		{"manifest persistence needs verification", &RunReport{Converge: &ConvergeReport{Outcome: "needs-verification"}}, runFlags{converge: true}, "unknown"},
		// NeedsVerification dominates the outcome: reporting `failed` for an apply
		// that succeeded invites re-applying it by hand.
		{"lost receipt outranks cap exhausted", &RunReport{Converge: &ConvergeReport{Outcome: "failed (cap_exhausted)", NeedsVerification: true}}, runFlags{converge: true}, "unknown"},
		{"lost receipt outranks execute error", &RunReport{Converge: &ConvergeReport{Outcome: "failed (execute_error)", NeedsVerification: true}}, runFlags{converge: true}, "unknown"},
		{"observe error after apply", &RunReport{Converge: &ConvergeReport{Outcome: "failed (observe_error)", NeedsVerification: true}}, runFlags{converge: true}, "unknown"},
		{"observe error before any apply", &RunReport{Converge: &ConvergeReport{Outcome: "failed (observe_error)"}}, runFlags{converge: true}, "failed"},
		{"dry-run writes nothing", &RunReport{Converge: &ConvergeReport{Outcome: "clean"}}, runFlags{converge: true, dryRun: true}, ""},
		{"verified drift clean -> clean", &RunReport{Drift: &ModelDriftResult{Clean: true, Verified: true}}, runFlags{}, "clean"},
		{"verified drift dirty", &RunReport{Drift: &ModelDriftResult{Clean: false, Verified: true}}, runFlags{}, "drifted"},
		// A catalog replay observed nothing, so it must not overwrite the verdict of
		// the model's last real observation — in either direction.
		{"replayed clean drift writes nothing", &RunReport{Drift: &ModelDriftResult{Clean: true}}, runFlags{fromCatalog: true, catalogScope: "snap"}, ""},
		{"replayed dirty drift writes nothing", &RunReport{Drift: &ModelDriftResult{Clean: false}}, runFlags{fromCatalog: true, catalogScope: "snap"}, ""},
		{"pure populate has no verdict", &RunReport{Populate: &PopulateReport{}}, runFlags{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, runVerdict(c.report, c.flags))
		})
	}
}

func TestModelRowVerdict(t *testing.T) {
	cases := []struct {
		name       string
		verdict    string
		restricted bool
		want       string
	}{
		// Unrestricted: the run covered the model, so its verdict is the model's.
		{"unscoped clean generalizes", "clean", false, "clean"},
		{"unscoped drifted generalizes", "drifted", false, "drifted"},
		{"unscoped failed generalizes", "failed", false, "failed"},
		{"unscoped unknown generalizes", "unknown", false, "unknown"},
		{"no verdict stays no verdict", "", false, ""},

		// Restricted: only `clean` fails to generalize from a subset to the model.
		// Writing it would let `pudl status`, `pudl model list` and
		// checkUpstreamFreshness read whole-model "in sync" off a partial run.
		{"scoped clean degrades to unknown", "clean", true, "unknown"},
		// Drift or failure in a subset is drift or failure in the model, so these
		// remain true statements about the whole and must not be weakened.
		{"scoped drifted still generalizes", "drifted", true, "drifted"},
		{"scoped failed still generalizes", "failed", true, "failed"},
		{"scoped unknown is already weakest", "unknown", true, "unknown"},
		{"scoped no verdict writes nothing", "", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, modelRowVerdict(c.verdict, c.restricted))
		})
	}
}

// End-to-end over a real catalog: a scoped clean run must leave the model's own
// status row `unknown` while the run row keeps the run's actual verdict and a
// note naming the scope. Without both halves the degraded `unknown` would be
// indistinguishable from the `unknown` of a lost manifest receipt.
func TestScopedCleanRun_ModelRowUnknown_RunRowKeepsVerdict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db, err := database.NewCatalogDB(config.GetPudlDir())
	require.NoError(t, err)

	// The model instance row `persistRunStatus` addresses.
	target := modelTargetKey("mymodel")
	entryType := "model-instance"
	require.NoError(t, db.AddEntry(database.CatalogEntry{
		ID: "model-row", StoredPath: "m.json", MetadataPath: "m.meta",
		ImportTimestamp: time.Now(), Format: "json", Origin: "t",
		Schema: "pudl/core.#SystemModel", Target: &target, EntryType: &entryType,
	}))
	db.Close()

	// A converge run of a subset that came back ∅.
	runID := "run_test_scoped"
	flags := runFlags{converge: true, only: []string{"web"}, maxIters: 5, onlySet: true}
	report := &RunReport{Converge: &ConvergeReport{Outcome: "clean"}}

	startRunRecord(runID, "mymodel", "converge", false)
	verdict := runVerdict(report, flags)
	require.Equal(t, "clean", verdict, "the run itself did reach ∅ over its scope")

	restricted := len(flags.only) > 0
	rowVerdict := modelRowVerdict(verdict, restricted)
	state := runFinishState{verdict: verdict, outcome: report.Converge.Outcome}
	if rowVerdict != verdict {
		state.note = fmt.Sprintf("verdict %q covers only the --only scope (%s); model status left %q",
			verdict, strings.Join(flags.only, ","), rowVerdict)
	}
	persistRunStatus("mymodel", rowVerdict, false)
	finishRunRecord(runID, state, nil, false)

	db2, err := database.NewCatalogDB(config.GetPudlDir())
	require.NoError(t, err)
	defer db2.Close()

	statuses, err := db2.GetTargetStatuses()
	require.NoError(t, err)
	got := map[string]string{}
	for _, s := range statuses {
		got[s.Target] = s.Status
	}
	assert.Equal(t, "unknown", got[target], "a scoped ∅ must not mark the whole model clean")

	run, err := db2.GetRun(runID)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "clean", run.Verdict, "the run row keeps what the run actually concluded")
	assert.True(t, run.Finished(), "the run is recorded terminal")
	assert.Contains(t, run.Note, "--only scope (web)", "the note names the scope that caused the divergence")
}

// The same path unscoped: a clean run does mark the model clean, so the
// degradation above is attributable to scope and not to a broken status write.
func TestUnscopedCleanRun_ModelRowClean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db, err := database.NewCatalogDB(config.GetPudlDir())
	require.NoError(t, err)
	target := modelTargetKey("mymodel")
	entryType := "model-instance"
	require.NoError(t, db.AddEntry(database.CatalogEntry{
		ID: "model-row", StoredPath: "m.json", MetadataPath: "m.meta",
		ImportTimestamp: time.Now(), Format: "json", Origin: "t",
		Schema: "pudl/core.#SystemModel", Target: &target, EntryType: &entryType,
	}))
	db.Close()

	flags := runFlags{converge: true, maxIters: 5}
	verdict := runVerdict(&RunReport{Converge: &ConvergeReport{Outcome: "clean"}}, flags)
	persistRunStatus("mymodel", modelRowVerdict(verdict, len(flags.only) > 0), false)

	db2, err := database.NewCatalogDB(config.GetPudlDir())
	require.NoError(t, err)
	defer db2.Close()
	statuses, err := db2.GetTargetStatuses()
	require.NoError(t, err)
	got := map[string]string{}
	for _, s := range statuses {
		got[s.Target] = s.Status
	}
	assert.Equal(t, "clean", got[target], "an unscoped ∅ does mark the model clean")
}

// Checks run off the effective scoped model (invariant 2), and the run guards on
// `len(effectiveModel.Checks)`. Scoping must therefore carry checks through: if
// it ever dropped them, that guard would silently skip every check on a scoped
// run rather than run them against the wrong model.
func TestScopeModelForRun_PreservesChecks(t *testing.T) {
	model := &systemmodel.SystemModel{
		Name:     "example",
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
		Checks: []systemmodel.Check{
			{Name: "no-orphans", Query: "orphan", Severity: "fail"},
		},
		Desired: []map[string]any{
			{"_schema": "pudl/k8s.#Deployment", "name": "web", "kind": "Deployment"},
			{"_schema": "pudl/k8s.#Service", "name": "api", "kind": "Service"},
		},
	}

	scoped, err := scopeModelForRun(model, []string{"web"})
	require.NoError(t, err)
	require.Len(t, scoped.Desired, 1, "scoping restricts desired")
	require.Len(t, scoped.Checks, 1, "scoping must not drop checks")
	assert.Equal(t, "no-orphans", scoped.Checks[0].Name)
	assert.Len(t, model.Checks, 1, "the original model is not mutated")
}
