package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunReportsRoundTripAndLatest(t *testing.T) {
	db := runsTestDB(t)

	first := []byte(`{"run_id":"run_a","ok":true}`)
	require.NoError(t, db.SaveRunReport("run_a", "model-a", first))
	got, err := db.GetRunReport("run_a")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "model-a", got.Model)
	assert.JSONEq(t, string(first), string(got.Report))

	// No sleep needed: LatestRunReport breaks created_at ties by run_id DESC,
	// so run_b is latest even when both reports share a timestamp.
	second := []byte(`{"run_id":"run_b","ok":false}`)
	require.NoError(t, db.SaveRunReport("run_b", "model-b", second))
	latest, err := db.LatestRunReport()
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "run_b", latest.RunID)

	missing, err := db.GetRunReport("missing")
	require.NoError(t, err)
	assert.Nil(t, missing)
}
