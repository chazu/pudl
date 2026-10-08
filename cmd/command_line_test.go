package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitCommandLine(t *testing.T) {
	cases := map[string][]string{
		`gcloud compute firewall-rules list --format=json`: {"gcloud", "compute", "firewall-rules", "list", "--format=json"},
		`a 'b c' "d \"e\" $HOME" f\ g`:                     {"a", "b c", `d "e" $HOME`, "f g"},
		`  x   ''  `:                                        {"x", ""},
	}
	for line, want := range cases {
		got, err := splitCommandLine(line)
		require.NoError(t, err, line)
		assert.Equal(t, want, got, line)
	}
	for _, bad := range []string{``, `a 'b`, `a "b`, `a \`} {
		_, err := splitCommandLine(bad)
		assert.Error(t, err, bad)
	}
}

func TestParsePopulateSpec(t *testing.T) {
	p, err := parsePopulateSpec("command:gcloud sql instances list --format=json")
	require.NoError(t, err)
	assert.Equal(t, []string{"gcloud", "sql", "instances", "list", "--format=json"}, p.argv)
	p, err = parsePopulateSpec("plugin:k8s")
	require.NoError(t, err)
	assert.Equal(t, "k8s", p.plugin)
	_, err = parsePopulateSpec("k8s")
	require.Error(t, err)
}
