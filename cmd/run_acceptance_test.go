package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The acceptance matrix from the architecture report, run without mu, without a
// cluster and without a network. Every phase reaches mu through the muRunner
// seam, so a scripted runner is all it takes.

// scriptedMu replays canned mu output. Each call pops the next scripted response
// for that operation, so a converge loop's successive observations can differ.
type scriptedMu struct {
	observes   [][]byte
	observed   int
	observeErr error

	manifests [][]byte
	applied   int
	applyErr  error

	plan     string
	buildLog []string
}

func (s *scriptedMu) Observe(configPath, target string) ([]byte, error) {
	if s.observeErr != nil {
		return nil, s.observeErr
	}
	if s.observed >= len(s.observes) {
		return nil, fmt.Errorf("unscripted observe #%d of %s", s.observed+1, target)
	}
	out := s.observes[s.observed]
	s.observed++
	return out, nil
}

func (s *scriptedMu) Build(configPath, target string, flags ...string) ([]byte, error) {
	s.buildLog = append(s.buildLog, fmt.Sprint(flags))
	for _, flag := range flags {
		if flag == "--plan" {
			return []byte(s.plan), nil
		}
	}
	if s.applyErr != nil {
		return nil, s.applyErr
	}
	if s.applied >= len(s.manifests) {
		return nil, fmt.Errorf("unscripted apply #%d of %s", s.applied+1, target)
	}
	out := s.manifests[s.applied]
	s.applied++
	return out, nil
}

const cleanObserve = `[{"target":"//models/m:converge","current":{"resources":[
	{"resource":"nginx","exists":true,"matches":true}
]}}]`

const driftedObserve = `[{"target":"//models/m:converge","current":{"resources":[
	{"resource":"nginx","exists":false,"matches":false}
]}}]`

func convergentModel() *systemmodel.SystemModel {
	return &systemmodel.SystemModel{
		Name:     "m",
		Plugins:  []systemmodel.PluginDef{{Name: "k8s", Command: []string{"true"}}},
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
		Desired: []map[string]any{
			{"_schema": "k8s.#Deployment", "name": "nginx", "kind": "Deployment"},
		},
	}
}

// acceptanceFixture stages a mu project root and a run-owned catalog.
func acceptanceFixture(t *testing.T) (*runCatalog, string) {
	t.Helper()
	cat := newRunCatalog(t.TempDir())
	t.Cleanup(func() { _ = cat.Close() })

	muRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(muRoot, "mu.cue"), []byte("package mu\n"), 0o644))
	return cat, muRoot
}

func startAcceptanceMutationRun(t *testing.T, cat *runCatalog) {
	t.Helper()
	db, err := cat.required()
	require.NoError(t, err)
	require.NoError(t, db.StartRun("run_a", "m", "converge"))
}

func TestAcceptance_ObserveOnlyDifferentialRun(t *testing.T) {
	cat, muRoot := acceptanceFixture(t)
	mu := &scriptedMu{observes: [][]byte{[]byte(cleanObserve)}}

	res, err := runDrift(cat, mu, convergentModel(), muRoot, t.TempDir(), "run_a")
	require.NoError(t, err)

	assert.True(t, res.Clean)
	assert.True(t, res.Verified, "a differential observe reads the live system")
	assert.Equal(t, 1, mu.observed, "mu observe was called")
	assert.Empty(t, mu.buildLog, "and nothing was applied")
	assert.NotEmpty(t, res.ObservationID, "the observation that decided the verdict is stored")
}

func TestAcceptance_ObserveOnlyDifferentialRunReportsDrift(t *testing.T) {
	cat, muRoot := acceptanceFixture(t)
	mu := &scriptedMu{observes: [][]byte{[]byte(driftedObserve)}}

	res, err := runDrift(cat, mu, convergentModel(), muRoot, t.TempDir(), "run_a")
	require.NoError(t, err)

	assert.False(t, res.Clean)
	require.Len(t, res.Drifted, 1)
	assert.Equal(t, "missing", res.Drifted[0].Reason)
}

func TestAcceptance_ConvergeToClean(t *testing.T) {
	// Drifted, apply, re-observe clean — the loop's whole point, and previously
	// untestable without a real cluster.
	cat, muRoot := acceptanceFixture(t)
	startAcceptanceMutationRun(t, cat)
	mu := &scriptedMu{
		observes:  [][]byte{[]byte(driftedObserve), []byte(cleanObserve)},
		manifests: [][]byte{[]byte(`{"actions":[]}`)},
	}

	report, err := runConvergeLoop(cat, mu, convergentModel(), muRoot, t.TempDir(), "run_a", 5, false, nil)
	require.NoError(t, err)
	require.Len(t, report.MutationReceipts, 1)
	assert.Equal(t, 1, report.MutationReceipts[0].Iteration)
	assert.Equal(t, "completed", report.MutationReceipts[0].Status)
	require.NotNil(t, report)

	assert.Equal(t, string(acute.OutcomeClean), report.Outcome)
	assert.Equal(t, 1, report.Iterations)
	assert.False(t, report.NeedsVerification)
	assert.Equal(t, 2, mu.observed, "observe, apply, re-observe")
	assert.Equal(t, 1, mu.applied)
}

func TestAcceptance_ApplyFailureIsNonCleanAndVisible(t *testing.T) {
	cat, muRoot := acceptanceFixture(t)
	startAcceptanceMutationRun(t, cat)
	mu := &scriptedMu{
		observes: [][]byte{[]byte(driftedObserve)},
		applyErr: fmt.Errorf("provider rejected the manifest"),
	}

	report, err := runConvergeLoop(cat, mu, convergentModel(), muRoot, t.TempDir(), "run_a", 5, false, nil)
	require.Error(t, err)
	require.NotNil(t, report)

	assert.Equal(t, string(acute.OutcomeExecuteError), report.Outcome)
	assert.Equal(t, 0, report.Iterations)
	assert.True(t, report.NeedsVerification)
	assert.Equal(t, "unknown", runVerdict(&RunReport{Converge: report}, runFlags{converge: true}))
}

func TestAcceptance_ManifestPersistenceFailureIsNotReportedAsClean(t *testing.T) {
	// The lost-receipt case: the apply succeeded and the re-observation is clean,
	// but the receipt could not be recorded. It must not become `clean`.
	cat, muRoot := acceptanceFixture(t)
	startAcceptanceMutationRun(t, cat)
	mu := &scriptedMu{
		observes:  [][]byte{[]byte(driftedObserve), []byte(cleanObserve)},
		manifests: [][]byte{[]byte(`not valid json — the ingest will reject it`)},
	}

	report, err := runConvergeLoop(cat, mu, convergentModel(), muRoot, t.TempDir(), "run_a", 5, false, nil)
	require.Error(t, err)
	require.NotNil(t, report)

	assert.True(t, report.NeedsVerification)
	assert.Equal(t, "unknown", runVerdict(&RunReport{Converge: report}, runFlags{converge: true}),
		"an apply that cannot be proven is unknown, not clean and not failed")
}

func TestAcceptance_DryRunPlansAndAppliesNothing(t *testing.T) {
	cat, muRoot := acceptanceFixture(t)
	mu := &scriptedMu{
		observes: [][]byte{[]byte(driftedObserve)},
		plan:     "would create nginx",
	}

	report, err := runConvergeLoop(cat, mu, convergentModel(), muRoot, t.TempDir(), "run_a", 5, true, nil)
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, string(acute.OutcomeDryRun), report.Outcome)
	assert.Equal(t, 0, mu.applied, "a dry run applies nothing")
	assert.Equal(t, []string{"[--plan --json]"}, mu.buildLog, "and only ever asks mu for a structured plan")
	assert.False(t, cat.opened, "nor does it open the catalog")
}

func TestAcceptance_ApplyBudgetStopsTheLoopWithoutApplying(t *testing.T) {
	cat, muRoot := acceptanceFixture(t)
	mu := &scriptedMu{observes: [][]byte{[]byte(driftedObserve)}}
	budget := 0

	report, err := runConvergeLoop(cat, mu, convergentModel(), muRoot, t.TempDir(), "run_a", 5, false, &budget)
	require.Error(t, err)
	require.NotNil(t, report)

	assert.Equal(t, string(acute.OutcomeBudgetExhausted), report.Outcome)
	assert.Equal(t, 1, mu.observed, "it still observes — that is how the budget resets")
	assert.Equal(t, 0, mu.applied, "and applies nothing")
}

func TestAcceptance_ObserveFailureAfterApplyNeedsVerification(t *testing.T) {
	// Applying and then failing to re-observe is the same operational state as a
	// lost receipt: the system changed and the result cannot be proven.
	cat, muRoot := acceptanceFixture(t)
	startAcceptanceMutationRun(t, cat)
	mu := &failAfterApplyMu{manifest: []byte(`{"actions":[]}`)}

	report, err := runConvergeLoop(cat, mu, convergentModel(), muRoot, t.TempDir(), "run_a", 5, false, nil)
	require.Error(t, err)
	require.NotNil(t, report)

	assert.True(t, report.NeedsVerification)
	assert.Equal(t, 1, report.Iterations)
	assert.Equal(t, "unknown", runVerdict(&RunReport{Converge: report}, runFlags{converge: true}))
}

// failAfterApplyMu observes dirty once, applies, then fails to observe.
type failAfterApplyMu struct {
	manifest []byte
	observed int
	applied  int
}

func (m *failAfterApplyMu) Observe(configPath, target string) ([]byte, error) {
	m.observed++
	if m.applied > 0 {
		return nil, fmt.Errorf("the cluster went away")
	}
	return []byte(driftedObserve), nil
}

func (m *failAfterApplyMu) Build(configPath, target string, flags ...string) ([]byte, error) {
	m.applied++
	return m.manifest, nil
}
