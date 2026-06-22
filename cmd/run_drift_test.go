package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterpretDifferentialObserve_Missing(t *testing.T) {
	// the real probe output: desired PDB not present in cluster.
	out := `[{"target":"//models/k8s:drift","current":{"resources":[
		{"exists":false,"resource":"PodDisruptionBudget/web"}
	]}}]`
	r, err := interpretDifferentialObserve([]byte(out))
	require.NoError(t, err)
	assert.False(t, r.Clean)
	require.Len(t, r.Drifted, 1)
	assert.Equal(t, "PodDisruptionBudget/web", r.Drifted[0].Resource)
	assert.Equal(t, "missing", r.Drifted[0].Reason)
}

func TestInterpretDifferentialObserve_Clean(t *testing.T) {
	out := `[{"target":"//m:drift","current":{"resources":[
		{"exists":true,"matches":true,"resource":"PodDisruptionBudget/web"},
		{"exists":true,"matches":true,"resource":"Deployment/api"}
	]}}]`
	r, err := interpretDifferentialObserve([]byte(out))
	require.NoError(t, err)
	assert.True(t, r.Clean)
	assert.Empty(t, r.Drifted)
}

func TestInterpretDifferentialObserve_Changed(t *testing.T) {
	out := `[{"target":"//m:drift","current":{"resources":[
		{"exists":true,"matches":false,"resource":"Deployment/api","diff":"replicas: 2 -> 3"}
	]}}]`
	r, err := interpretDifferentialObserve([]byte(out))
	require.NoError(t, err)
	assert.False(t, r.Clean)
	require.Len(t, r.Drifted, 1)
	assert.Equal(t, "changed", r.Drifted[0].Reason)
	assert.Equal(t, "replicas: 2 -> 3", r.Drifted[0].Diff)
}

func TestInterpretDifferentialObserve_Mixed(t *testing.T) {
	out := `[{"target":"//m:drift","current":{"resources":[
		{"exists":true,"matches":true,"resource":"A/ok"},
		{"exists":false,"resource":"B/gone"},
		{"exists":true,"matches":false,"resource":"C/stale","diff":"x"}
	]}}]`
	r, err := interpretDifferentialObserve([]byte(out))
	require.NoError(t, err)
	assert.False(t, r.Clean)
	assert.Len(t, r.Drifted, 2) // ok is clean; gone + stale drift
}

func TestInterpretDifferentialObserve_Error(t *testing.T) {
	out := `[{"target":"//m:drift","error":"observe failed: no cluster"}]`
	_, err := interpretDifferentialObserve([]byte(out))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no cluster")
}
