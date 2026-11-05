package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func packStates(t *testing.T) map[string]string {
	t.Helper()
	r := runCLI(t, "pack", "list", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var out struct {
		Packs []struct{ Name, State string }
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
	states := map[string]string{}
	for _, p := range out.Packs {
		states[p.Name] = p.State
	}
	return states
}

func TestPackListShowsInstallState(t *testing.T) {
	ws := cliWorkspace(t)
	require.Equal(t, map[string]string{"gcp": "not-installed", "aws": "not-installed"}, packStates(t))

	r := runCLI(t, "pack", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "Next: pudl check gcp-fixture-firewalls")
	require.FileExists(t, filepath.Join(ws, ".packs", "gcp.json"))
	require.Equal(t, map[string]string{"gcp": "installed", "aws": "not-installed"}, packStates(t))

	r = runCLI(t, "pack", "list")
	require.NoError(t, r.Err)
	require.Regexp(t, `gcp\s+installed`, r.Stdout)
}

// The packs left pudl example: there is no alias, and an install recorded
// under .examples is not migrated.
func TestExampleInstallNoLongerKnowsPacks(t *testing.T) {
	ws := cliWorkspace(t)
	for _, name := range []string{"gcp", "aws"} {
		r := runCLI(t, "example", "install", name)
		require.ErrorContains(t, r.Err, `unknown example "`+name+`"`)
	}
	require.NoDirExists(t, filepath.Join(ws, "schema", "gcp"))

	r := runCLI(t, "pack", "install", "gcp")
	require.NoError(t, r.Err)
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".examples"), 0o755))
	require.NoError(t, os.Rename(filepath.Join(ws, ".packs", "gcp.json"), filepath.Join(ws, ".examples", "gcp.json")))
	require.Equal(t, "not-installed", packStates(t)["gcp"])
}
