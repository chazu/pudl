package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inputSnapshot is one input's observation of run in scope "k8s/<input>/x".
func inputSnapshot(id, run, input string, at time.Time) ObserveSnapshot {
	s := snapshotFor(id, "m", SnapshotSourceCommand, at)
	s.RunID, s.Input, s.Scope, s.Complete = run, input, "k8s/"+input+"/x", true
	return s
}

func TestObservationInputRoundTrips(t *testing.T) {
	db := snapshotTestDB(t)
	require.NoError(t, db.RecordObserveSnapshot(inputSnapshot("snap_1", "run_1", "a", time.Now())))
	got, err := db.GetObserveSnapshot("snap_1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "a", got.Input)
	assert.Equal(t, "k8s/a/x", got.Scope)
}

// The latest observation of a per-input model is every input snapshot of its
// newest run; earlier runs' inputs are not part of it. A fixed-scope model's
// is its newest snapshot.
func TestModelObservations(t *testing.T) {
	db := snapshotTestDB(t)
	at := time.Now().Add(-time.Hour)
	for i, s := range []ObserveSnapshot{
		inputSnapshot("old_a", "run_1", "a", at),
		inputSnapshot("new_b", "run_2", "b", at.Add(time.Minute)),
		inputSnapshot("new_c", "run_2", "c", at.Add(time.Minute)),
	} {
		require.NoError(t, db.RecordObserveSnapshot(s), i)
	}
	got, err := db.ModelObservations("m")
	require.NoError(t, err)
	var ids []string
	for _, s := range got {
		ids = append(ids, s.SnapshotID)
	}
	assert.Equal(t, []string{"new_b", "new_c"}, ids)

	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("fixed_1", "f", SnapshotSourceCommand, at)))
	require.NoError(t, db.RecordObserveSnapshot(snapshotFor("fixed_2", "f", SnapshotSourceCommand, at.Add(time.Minute))))
	got, err = db.ModelObservations("f")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "fixed_2", got[0].SnapshotID)

	got, err = db.ModelObservations("none")
	require.NoError(t, err)
	assert.Empty(t, got)
}

// The previous observation of an input is the previous one of its scope, not
// another input's.
func TestPreviousSnapshotOfAnInputIsOfItsScope(t *testing.T) {
	db := snapshotTestDB(t)
	at := time.Now().Add(-time.Hour)
	for i, s := range []ObserveSnapshot{
		inputSnapshot("a_1", "run_1", "a", at),
		inputSnapshot("b_1", "run_2", "b", at.Add(time.Minute)),
		inputSnapshot("a_2", "run_3", "a", at.Add(2*time.Minute)),
	} {
		require.NoError(t, db.RecordObserveSnapshot(s), i)
		finishSnapshotRun(t, db, s, RunStatusSucceeded)
	}
	previous, _, err := db.PreviousSuccessfulObserveSnapshot(context.Background(), "a_2")
	require.NoError(t, err)
	require.NotNil(t, previous)
	assert.Equal(t, "a_1", previous.SnapshotID)
}

// Pruning keeps the newest successful observation of every input's scope, and
// a newer per-input observation stops protecting the model's fixed-scope one.
func TestPruneKeepsTheLatestObservationOfEachInput(t *testing.T) {
	db, dataDir, raw := pruneFixture(t)
	at := time.Now().Add(-10 * time.Hour)
	fixed := snapshotFor("fixed", "m", SnapshotSourceCommand, at)
	fixed.Scope = "k8s/x"
	seedSnapshot(t, db, fixed, raw, "fixed_rec")
	finishSnapshotRun(t, db, fixed, RunStatusSucceeded)
	for _, s := range []ObserveSnapshot{
		inputSnapshot("a_1", "run_1", "a", at.Add(time.Hour)),
		inputSnapshot("b_1", "run_2", "b", at.Add(2*time.Hour)),
		inputSnapshot("b_2", "run_3", "b", at.Add(3*time.Hour)),
	} {
		seedSnapshot(t, db, s, raw, s.SnapshotID+"_rec")
		finishSnapshotRun(t, db, s, RunStatusSucceeded)
	}
	result, err := db.PruneObserveSnapshots(PruneOptions{Model: "m", Keep: 0, DataDir: dataDir, DryRun: true})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"fixed", "b_1"}, result.Snapshots)
}
