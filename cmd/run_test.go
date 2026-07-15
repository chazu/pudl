package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		{"dry-run writes nothing", &RunReport{Converge: &ConvergeReport{Outcome: "clean"}}, runFlags{converge: true, dryRun: true}, ""},
		{"drift clean -> clean", &RunReport{Drift: &ModelDriftResult{Clean: true}}, runFlags{}, "clean"},
		{"drift dirty", &RunReport{Drift: &ModelDriftResult{Clean: false}}, runFlags{}, "drifted"},
		{"pure populate has no verdict", &RunReport{Populate: &PopulateReport{}}, runFlags{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, runVerdict(c.report, c.flags))
		})
	}
}
