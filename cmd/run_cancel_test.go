package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Interrupting a run: the real mu subprocess path (execMu) is stopped, the run
// records a truthful conclusion, and no workspace is left in the mu project.

// installFakeMu puts a `mu` on PATH that observes drift, answers --plan, and
// blocks in apply until it is terminated, announcing the apply via a marker.
func installFakeMu(t *testing.T) (applyStarted, terminated string) {
	t.Helper()
	dir := t.TempDir()
	observe := filepath.Join(dir, "observe.json")
	require.NoError(t, os.WriteFile(observe, []byte(driftedObserve), 0o644))
	applyStarted = filepath.Join(dir, "apply-started")
	terminated = filepath.Join(dir, "terminated")
	script := `#!/bin/sh
case "$1" in
observe) cat "` + observe + `" ;;
build)
  for a in "$@"; do
    if [ "$a" = "--plan" ]; then echo '{}'; exit 0; fi
  done
  trap 'touch "` + terminated + `"; exit 143' TERM
  touch "` + applyStarted + `"
  while :; do sleep 0.05; done ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mu"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return applyStarted, terminated
}

func quietRun(t *testing.T) {
	t.Helper()
	previous := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previous })
}

func pudlRunWorkspaces(t *testing.T, muRoot string) []string {
	t.Helper()
	entries, err := os.ReadDir(muRoot)
	require.NoError(t, err)
	var found []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), workspacePrefix) {
			found = append(found, entry.Name())
		}
	}
	return found
}

func TestConvergeInterruptedMidApplyConcludesCancelledAndNeedsVerification(t *testing.T) {
	quietRun(t)
	applyStarted, terminated := installFakeMu(t)
	cat, muRoot := acceptanceFixture(t)
	startAcceptanceMutationRun(t, cat)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for i := 0; i < 400; i++ {
			if _, err := os.Stat(applyStarted); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	rep, runErr := runConvergeLoop(cat, newExecMu(ctx, 0), convergentModel(), muRoot, t.TempDir(), "run_a", 3, false, nil)

	require.Error(t, runErr)
	assert.True(t, errors.Is(runErr, context.Canceled), "the run error carries the cancellation: %v", runErr)
	_, err := os.Stat(terminated)
	assert.NoError(t, err, "mu was sent SIGTERM so it could finish writing its receipt")
	require.NotNil(t, rep)
	assert.True(t, rep.NeedsVerification, "an apply was in flight, so the system state is unproven")

	conclusion := runConclusion(runFinishState{outcome: rep.Outcome, needsVerification: rep.NeedsVerification}, runErr)
	assert.Equal(t, database.RunStatusCancelled, conclusion.CompletionStatus)
	assert.True(t, conclusion.NeedsVerification)

	db, err := cat.required()
	require.NoError(t, err)
	require.NoError(t, db.FinishRun("run_a", conclusion))
	run, err := db.GetRun("run_a")
	require.NoError(t, err)
	assert.Equal(t, database.RunStatusCancelled, run.CompletionStatus)
	assert.True(t, run.NeedsVerification)

	assert.Empty(t, pudlRunWorkspaces(t, muRoot), "the reconcile workspace was removed on the way out")
	assert.Empty(t, workspaces.tracked(), "nothing is left registered")
}

func TestMuTimeoutFailsRatherThanCancels(t *testing.T) {
	quietRun(t)
	installFakeMu(t)
	cat, muRoot := acceptanceFixture(t)
	startAcceptanceMutationRun(t, cat)

	rep, runErr := runConvergeLoop(cat, newExecMu(context.Background(), 300*time.Millisecond),
		convergentModel(), muRoot, t.TempDir(), "run_a", 3, false, nil)

	require.Error(t, runErr)
	assert.True(t, errors.Is(runErr, context.DeadlineExceeded), "%v", runErr)
	assert.Contains(t, runErr.Error(), "timed out after 300ms")
	require.NotNil(t, rep)
	assert.True(t, rep.NeedsVerification, "a timed-out apply may still have mutated the system")
	assert.Equal(t, database.RunStatusFailed, failureStatus(runErr), "nobody asked the run to stop")
}

func TestInterruptedRunSetStartsNoFurtherMutation(t *testing.T) {
	db, member, mu := setConvergeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report := newSingleMemberSetReport(member)
	err := executePreparedMutationPlan(ctx, db, mu, report, singleMemberPlan(member),
		map[string]*preparedMutationMember{member.model.Name: member})

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Equal(t, database.RunStatusCancelled, report.Status)
	require.Len(t, report.Members, 1)
	assert.Equal(t, database.RunStatusCancelled, report.Members[0].Result)
	assert.Empty(t, mu.buildLog, "no apply was attempted after the interrupt")
	assert.Zero(t, mu.observed, "the member did not start")
}
