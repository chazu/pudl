//go:build smoke

package smoke

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSmoke_AgentMemoryRemovedGenericFactsRetained(t *testing.T) {
	root, _, env := exampleWorkspace(t, "git")
	cli := func(args ...string) []byte {
		t.Helper()
		command := exec.Command(pudlBin, args...)
		command.Dir, command.Env = root, env
		out, err := command.CombinedOutput()
		require.NoError(t, err, "%v: %s", args, out)
		return out
	}
	cli("init")
	require.NoFileExists(t, filepath.Join(root, ".pudl/schema/pudl/nous/nous.cue"))
	require.NoFileExists(t, filepath.Join(root, ".pudl/mu.cue"))
	for _, args := range [][]string{{"memory", "context"}, {"memory", "cycle"}, {"hooks", "install"}, {"pull", "demo"}, {"facts", "observe", "demo"}, {"facts", "promote", "demo"}, {"facts", "curate"}, {"guide", "memory"}} {
		command := exec.Command(pudlBin, args...)
		command.Dir, command.Env = root, env
		out, err := command.CombinedOutput()
		require.Error(t, err, "retired command should fail: %v: %s", args, out)
	}
	for _, args := range [][]string{{"prime"}, {"guide"}, {"guide", "agents"}, {"guide", "facts"}, {"help", "--json"}} {
		out := string(cli(args...))
		for _, obsolete := range []string{"pudl memory", "pudl hooks", "pudl facts observe", "pudl facts promote", "pudl facts curate", "pudl pull"} {
			require.NotContains(t, out, obsolete)
		}
	}
	var fact struct {
		ID       string `json:"id"`
		Relation string `json:"relation"`
		Args     string `json:"args"`
	}
	cliArgs := []string{"--json", "facts", "add", "--relation", "config", "--args", `{"key":"timeout","value":30}`, "--source", "operator"}
	require.NoError(t, json.Unmarshal(cli(cliArgs...), &fact))
	require.Equal(t, "config", fact.Relation)
	require.NotEmpty(t, fact.ID)
	var matches []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(cli("facts", "search", "timeout", "--json"), &matches))
	require.Len(t, matches, 1)
	require.Equal(t, fact.ID, matches[0].ID)
	var stats []map[string]any
	require.NoError(t, json.Unmarshal(cli("facts", "stats", "--relation", "config", "--json"), &stats))
	require.Len(t, stats, 1)
	require.Equal(t, "config", stats[0]["relation"])
	// Legacy relation names are still ordinary data, without forced maturity fields.
	require.NoError(t, json.Unmarshal(cli("facts", "add", "--relation", "observation", "--args", `{"description":"legacy evidence"}`, "--json"), &fact))
	require.JSONEq(t, `{"description":"legacy evidence"}`, fact.Args)
	cli("facts", "retract", fact.ID)
	require.NoError(t, json.Unmarshal(cli("facts", "search", "legacy", "--json"), &matches))
	require.Empty(t, matches)
	// Raw historical evidence remains accessible after retraction.
	require.NoError(t, json.Unmarshal(cli("facts", "show", fact.ID, "--json"), &fact))
	require.JSONEq(t, `{"description":"legacy evidence"}`, fact.Args)
	_, err := os.Stat(filepath.Join(root, ".pudl/mu.cue"))
	require.True(t, os.IsNotExist(err))
}
