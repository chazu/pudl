package systemmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitCommandLine(t *testing.T) {
	cases := map[string][]string{
		`gcloud compute firewall-rules list --format=json`: {"gcloud", "compute", "firewall-rules", "list", "--format=json"},
		`a 'b c' "d \"e\" $HOME" f\ g`:                     {"a", "b c", `d "e" $HOME`, "f g"},
		`  x   ''  `:                                       {"x", ""},
	}
	for line, want := range cases {
		got, err := SplitCommandLine(line)
		require.NoError(t, err, line)
		assert.Equal(t, want, got, line)
	}
	for _, bad := range []string{``, `a 'b`, `a "b`, `a \`} {
		_, err := SplitCommandLine(bad)
		assert.Error(t, err, bad)
	}
}

func TestParsePopulateSpec(t *testing.T) {
	argv, err := ParsePopulateSpec("command:gcloud sql instances list --format=json")
	require.NoError(t, err)
	assert.Equal(t, []string{"gcloud", "sql", "instances", "list", "--format=json"}, argv)
	_, err = ParsePopulateSpec("plugin:k8s")
	require.ErrorContains(t, err, "retired")
	_, err = ParsePopulateSpec("k8s")
	require.Error(t, err)
}

func TestAdHocModelIsNamedAfterItsProgram(t *testing.T) {
	m, err := AdHocModel("command:/usr/bin/gcloud compute networks list")
	require.NoError(t, err)
	assert.Equal(t, "adhoc-gcloud", m.Name)
	assert.Equal(t, []CommandRun{{Argv: []string{"/usr/bin/gcloud", "compute", "networks", "list"}}}, m.Populate.Runs)
}
