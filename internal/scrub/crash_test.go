package scrub

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrubCrashBeforeCommitLeavesTheCatalogUnchanged(t *testing.T) {
	f := newFixture(t)
	doc := f.importFile("solo.json", `{"name":"solo","password":"`+secretC+`"}`)
	f.declare()
	s, db := f.scrubber()
	snapshot := f.state(db)

	// Simulate a process killed after publishing the new files and before the
	// commit: keep copies of what it published, so they survive the rollback
	// the way they would survive a SIGKILL.
	kept := map[string][]byte{}
	beforeCommit = func(published []string) error {
		for _, path := range published {
			b, err := os.ReadFile(path)
			require.NoError(t, err)
			kept[path] = b
		}
		return fmt.Errorf("simulated crash")
	}
	t.Cleanup(func() { beforeCommit = nil })
	_, err := s.Apply(context.Background(), false)
	require.ErrorContains(t, err, "simulated crash")
	assert.Equal(t, snapshot, f.state(db), "the catalog and the old files are untouched")
	require.NotEmpty(t, kept)
	for path, b := range kept {
		require.NoError(t, os.WriteFile(path, b, 0o600))
	}

	beforeCommit = nil
	outcome, err := s.Apply(context.Background(), false)
	require.NoError(t, err, "the rerun reuses the published files")
	require.Empty(t, outcome.Errors)
	assert.Empty(t, f.plaintextFiles(secretC))
	assert.FileExists(t, entryIDs(t, db)[mustResolve(t, db, doc.ID)].StoredPath)
}

func TestScrubCrashAfterCommitIsFinishedByTheNextRun(t *testing.T) {
	f := newFixture(t)
	doc := f.importFile("solo.json", `{"name":"solo","password":"`+secretC+`"}`)
	f.declare()
	s, _ := f.scrubber()

	afterCommit = func() error { return fmt.Errorf("simulated crash") }
	t.Cleanup(func() { afterCommit = nil })
	_, err := s.Apply(context.Background(), false)
	require.ErrorContains(t, err, "simulated crash")
	afterCommit = nil

	plan, err := s.Plan(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{doc.StoredPath, doc.MetadataPath}, plan.Leftovers, "superseded files are reported")
	assert.Len(t, plan.Uncompacted, 1)
	assert.Empty(t, plan.Subjects())

	outcome, err := s.Apply(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, 3, outcome.Changed, "two files unlinked and one compaction finished")
	assert.Empty(t, f.plaintextFiles(secretC))
	again, err := s.Plan(context.Background())
	require.NoError(t, err)
	assert.Zero(t, again.Pending())
}
