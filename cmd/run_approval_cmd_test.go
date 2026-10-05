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

func TestResumedRunOptionsRestoreExplicitMuRoot(t *testing.T) {
	opts := resumedRunOptions("run_1", approvalRequest{
		Model: "model", Only: []string{"one"}, MaxIters: 3, MaxApplies: 7,
		MuRoot: "/repo/.pudl/data/mu",
	})
	assert.Equal(t, "model", opts.model)
	assert.Equal(t, []string{"one"}, opts.only)
	assert.Equal(t, 3, opts.maxIters)
	assert.Equal(t, 7, opts.maxApplies)
	assert.Equal(t, "/repo/.pudl/data/mu", opts.muRoot)
	assert.True(t, opts.converge)
	assert.False(t, opts.requireApproval)
	assert.Equal(t, "run_1", opts.resumeID)
	assert.Equal(t, "approved", opts.approvalStatus)
	require.NoError(t, validateRunFlags(opts.runFlags))
}
