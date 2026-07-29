package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunApprovalsRoundTripAndTerminalState(t *testing.T) {
	db := runsTestDB(t)
	request := []byte(`{"model":"web","max_iters":3,"max_applies":1}`)

	require.NoError(t, db.SaveRunApproval("run_a", "web", request))
	got, err := db.GetRunApproval("run_a")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "web", got.Model)
	assert.Equal(t, "pending", got.Status)
	assert.JSONEq(t, string(request), string(got.Request))
	assert.Nil(t, got.ResolvedAt)

	require.NoError(t, db.ResolveRunApproval("run_a", "approved"))
	got, err = db.GetRunApproval("run_a")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "approved", got.Status)
	assert.NotNil(t, got.ResolvedAt)
	assert.Error(t, db.ResolveRunApproval("run_a", "rejected"), "terminal approvals cannot be resolved twice")
}
