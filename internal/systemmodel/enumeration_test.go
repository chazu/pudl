package systemmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadModel_EachAndFailFast(t *testing.T) {
	m, err := loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	populate: {
		fail_fast: true
		each: {argv: ["gcloud", "projects", "list", "--format=json"], select: "projectId"}
		runs: [{argv: ["gcloud", "compute", "firewall-rules", "list", "--project={{item}}"], set: project: "{{item}}"}]
	}
}
`), "fw")
	require.NoError(t, err)
	assert.True(t, m.Populate.FailFast)
	require.NotNil(t, m.Populate.Each)
	assert.Equal(t, Enumeration{Argv: []string{"gcloud", "projects", "list", "--format=json"}, Select: "projectId"}, *m.Populate.Each)

	plain, err := loadModel([]byte(commandObserveModel), "pods")
	require.NoError(t, err)
	assert.False(t, plain.Populate.FailFast, "fail_fast defaults to false")
	assert.Nil(t, plain.Populate.Each)
}

func TestLoadModel_ItemPlaceholderMustMatchEach(t *testing.T) {
	_, err := loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	populate: {
		each: argv: ["list-projects"]
		runs: [{argv: ["list", "--project={{item}}"]}, {argv: ["list-global"]}]
	}
}
`), "fw")
	require.ErrorContains(t, err, "populate run 1 does not use {{item}}")

	_, err = loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	populate: runs: [{argv: ["list"], set: project: "{{item}}"}]
}
`), "fw")
	require.ErrorContains(t, err, "populate run 0 uses {{item}} but the populate declares no `each`")
}

func TestCommandRunForItemSubstitutesArgvAndSet(t *testing.T) {
	run := CommandRun{
		Argv: []string{"list", "--project={{item}}"},
		Set:  map[string]any{"project": "{{item}}", "labels": map[string]any{"src": "x-{{item}}"}, "n": 1},
		Dir:  "d",
	}
	got := run.ForItem("p1")
	assert.Equal(t, CommandRun{
		Argv: []string{"list", "--project=p1"},
		Set:  map[string]any{"project": "p1", "labels": map[string]any{"src": "x-p1"}, "n": 1},
		Dir:  "d",
	}, got)
	assert.Equal(t, "--project={{item}}", run.Argv[1], "the template is unchanged")
}
