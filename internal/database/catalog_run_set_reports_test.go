package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSetReportsRoundTripAndLatest(t *testing.T) {
	db := snapshotTestDB(t)
	require.NoError(t, db.SaveRunSetReport("set_a", []byte(`{"status":"running"}`)))
	require.NoError(t, db.SaveRunSetReport("set_a", []byte(`{"status":"succeeded"}`)))
	require.NoError(t, db.SaveRunSetReport("set_b", []byte(`{"status":"failed"}`)))

	first, err := db.GetRunSetReport("set_a")
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.JSONEq(t, `{"status":"succeeded"}`, string(first.Report))

	latest, err := db.LatestRunSetReport()
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "set_b", latest.RunSetID)

	missing, err := db.GetRunSetReport("missing")
	require.NoError(t, err)
	assert.Nil(t, missing)
}
