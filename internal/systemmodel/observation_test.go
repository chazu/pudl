package systemmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadModel_PerInputScope(t *testing.T) {
	m, err := loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	observation: {scope: "gcp/{{item}}/firewalls", complete: true}
	populate: {
		each: argv: ["list-projects"]
		runs: [{argv: ["list", "--project={{item}}"]}]
	}
}
`), "fw")
	require.NoError(t, err)
	assert.True(t, m.Observation.PerInput())
	assert.Equal(t, "gcp/p1/firewalls", m.Observation.ScopeFor("p1"))

	fixed := Observation{Scope: "gcp/firewalls"}
	assert.False(t, fixed.PerInput())
	assert.Equal(t, "gcp/firewalls", fixed.ScopeFor("p1"), "a fixed scope is unchanged")
	var none *Observation
	assert.False(t, none.PerInput())
}

func TestLoadModel_PerInputScopeNeedsEachWithoutFallbackOrDesired(t *testing.T) {
	_, err := loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	observation: {scope: "gcp/{{item}}/firewalls", complete: true}
	populate: runs: [{argv: ["list"]}]
}
`), "fw")
	require.ErrorContains(t, err, "the populate declares no `each`")

	_, err = loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	observation: {scope: "gcp/{{item}}/firewalls", complete: true}
	populate: {
		each: argv: ["list-projects"]
		runs: [{argv: ["list", "--project={{item}}"]}]
		fallback: {when: ["DENIED"], observe: runs: [{argv: ["list-all"]}]}
	}
}
`), "fw")
	require.ErrorContains(t, err, "a populate with a fallback cannot use")

	_, err = loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	observation: {scope: "gcp/{{item}}/firewalls", complete: true}
	populate: {
		each: argv: ["list-projects"]
		runs: [{argv: ["list", "--project={{item}}"]}]
	}
	desired: [{"_schema": "gcp.firewall", name: "a"}]
}
`), "fw")
	require.ErrorContains(t, err, "a model with desired cannot use")
}
