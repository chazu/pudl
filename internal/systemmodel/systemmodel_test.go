package systemmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// observe-only k8s model — the V1 demo-path populate case.
const k8sObserveModel = `
k8sPolicy: #SystemModel & {
	name: "k8s-policy"
	populate: #PluginObserve & {
		plugin: "k8s"
		input: {namespace: "default", context: "prod"}
	}
	checks: [{
		name:     "pdb_present"
		query:    "missing_pdb"
		expect:   "empty"
		severity: "warn"
		message:  "workload without PodDisruptionBudget"
	}]
}
`

// convergence k8s model — adds desired + a #PluginPlan converge arm.
const k8sConvergeModel = `
k8sConverge: #SystemModel & {
	name: "k8s-converge"
	populate: #PluginObserve & {
		plugin: "k8s"
		input: {namespace: "default"}
	}
	desired: [
		{apiVersion: "policy/v1", kind: "PodDisruptionBudget", metadata: {name: "web"}},
	]
	converge: #PluginPlan & {
		plugin: "k8s"
		input: {namespace: "default", apply: ["PodDisruptionBudget"]}
	}
}
`

func TestLoadModel_ObserveOnly(t *testing.T) {
	m, err := LoadModel([]byte(k8sObserveModel), "k8sPolicy")
	require.NoError(t, err)

	assert.Equal(t, "k8s-policy", m.Name)

	// populate dispatches to the observe arm.
	assert.Equal(t, KindPluginObserve, m.Populate.Kind())
	assert.Equal(t, "k8s", m.Populate.Plugin)
	assert.Equal(t, "default", m.Populate.Input["namespace"])
	assert.Equal(t, "prod", m.Populate.Input["context"])

	// one check, decoded.
	require.Len(t, m.Checks, 1)
	assert.Equal(t, "pdb_present", m.Checks[0].Name)
	assert.Equal(t, "missing_pdb", m.Checks[0].Query)
	assert.Equal(t, "empty", m.Checks[0].Expect)
	assert.Equal(t, "warn", m.Checks[0].Severity)

	// observe-only: no converge arm.
	assert.False(t, m.Convergent())
	assert.Nil(t, m.Converge)
}

func TestLoadModel_Convergent(t *testing.T) {
	m, err := LoadModel([]byte(k8sConvergeModel), "k8sConverge")
	require.NoError(t, err)

	assert.Equal(t, "k8s-converge", m.Name)
	assert.Equal(t, KindPluginObserve, m.Populate.Kind())

	// desired is present and decoded.
	require.Len(t, m.Desired, 1)
	assert.Equal(t, "PodDisruptionBudget", m.Desired[0]["kind"])

	// convergence: the #PluginPlan arm decodes.
	require.True(t, m.Convergent())
	require.NotNil(t, m.Converge)
	assert.Equal(t, "k8s", m.Converge.Plugin)
	assert.Equal(t, "default", m.Converge.Input["namespace"])
}

func TestLoadModel_MissingInstance(t *testing.T) {
	_, err := LoadModel([]byte(k8sObserveModel), "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLoadModel_RejectsInvalid(t *testing.T) {
	// populate is required; a model without it must not load.
	const bad = `bad: #SystemModel & {name: "x"}`
	_, err := LoadModel([]byte(bad), "bad")
	require.Error(t, err)
}
