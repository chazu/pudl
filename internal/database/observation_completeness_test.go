package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestObservationScopeNeverMixesOwners(t *testing.T) {
	db := snapshotTestDB(t)
	first := ObserveSnapshot{SnapshotID: "first", Model: "a", Scope: "prod", Complete: true, Source: SnapshotSourceCommand, CreatedAt: time.Now().Add(-time.Minute)}
	require.NoError(t, db.RecordObserveSnapshot(first))
	partial := first
	partial.SnapshotID = "partial"
	partial.Complete = false
	partial.CreatedAt = time.Now()
	require.NoError(t, db.RecordObserveSnapshot(partial))
	selected, err := db.CurrentScopeSnapshot("prod")
	require.NoError(t, err)
	require.Equal(t, "partial", selected.SnapshotID)
	require.False(t, selected.Complete)
	other := first
	other.SnapshotID = "other"
	other.Model = "b"
	require.NoError(t, db.RecordObserveSnapshot(other))
	_, err = db.CurrentScopeSnapshot("prod")
	require.ErrorContains(t, err, "multiple observation owners")
	old, err := db.GetObserveSnapshot("first")
	require.NoError(t, err)
	require.True(t, old.Complete)
}
