package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApprovalRequestPreservesExplicitMuRoot(t *testing.T) {
	request := newApprovalRequest("model", runFlags{
		only: []string{"one"}, maxIters: 3, maxApplies: 7,
	}, "/repo/.pudl/data/mu")
	payload, err := json.Marshal(request)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"model",
		"only":["one"],
		"max_iters":3,
		"max_applies":7,
		"mu_root":"/repo/.pudl/data/mu"
	}`, string(payload))
}

func TestRestoreApprovalRequestRestoresExplicitMuRoot(t *testing.T) {
	previousOnly, previousIters := runOnly, runMaxIters
	previousApplies, previousMuRoot := runMaxApplies, runMuRoot
	t.Cleanup(func() {
		runOnly, runMaxIters = previousOnly, previousIters
		runMaxApplies, runMuRoot = previousApplies, previousMuRoot
	})

	restoreApprovalRequest(approvalRequest{
		Only: []string{"one"}, MaxIters: 3, MaxApplies: 7,
		MuRoot: "/repo/.pudl/data/mu",
	})
	assert.Equal(t, []string{"one"}, runOnly)
	assert.Equal(t, 3, runMaxIters)
	assert.Equal(t, 7, runMaxApplies)
	assert.Equal(t, "/repo/.pudl/data/mu", runMuRoot)
}
