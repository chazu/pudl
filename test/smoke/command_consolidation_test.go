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

func TestSmoke_ConsolidatedCommandSurface(t *testing.T) {
	root, sandbox, env := exampleWorkspace(t, "git")
	cli := func(args ...string) []byte {
		t.Helper()
		command := exec.Command(pudlBin, args...)
		command.Dir, command.Env = root, env
		output, err := command.CombinedOutput()
		require.NoError(t, err, "%v: %s", args, output)
		return output
	}
	// Discovery must stay side-effect free with flags preceding the command.
	var tree map[string]any
	require.NoError(t, json.Unmarshal(cli("--json", "help"), &tree))
	global := filepath.Join(sandbox, "home", ".pudl")
	_, err := os.Stat(global)
	require.True(t, os.IsNotExist(err))
	var initResult map[string]string
	require.NoError(t, json.Unmarshal(cli("--json", "init"), &initResult))
	require.Equal(t, "workspace", initResult["mode"])
	require.Equal(t, filepath.Join(root, ".pudl"), initResult["path"])
	cli("init")
	_, err = os.Stat(global)
	require.True(t, os.IsNotExist(err), "local initialization must not create global state")
	cli("example", "install", "git-inventory")
	for _, args := range [][]string{
		{"--json", "model", "show", "git-inventory"},
		{"model", "show", "git-inventory", "--json"},
	} {
		var model map[string]any
		require.NoError(t, json.Unmarshal(cli(args...), &model))
		require.Equal(t, "git-inventory", model["name"])
		require.NotNil(t, model["populate"])
		require.NotNil(t, model["desired"])
	}
	var models []map[string]any
	require.NoError(t, json.Unmarshal(cli("--json", "model", "list"), &models))
	require.Len(t, models, 1)
	var schemas []struct {
		FullName string `json:"full_name"`
		Metadata struct {
			ResourceType string `json:"resource_type"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(cli("--json", "schema", "list", "--package", "pudl/git"), &schemas))
	require.NotEmpty(t, schemas)
	found := false
	for _, schema := range schemas {
		if schema.FullName == "pudl/git.#GitRepository" {
			found = true
			require.Equal(t, "git.repository", schema.Metadata.ResourceType)
		}
	}
	require.True(t, found)
	var doctor struct {
		OK bool `json:"ok"`
	}
	require.NoError(t, json.Unmarshal(cli("--json", "doctor"), &doctor))
	require.True(t, doctor.OK)
	// Old commands are removed, not retained as another discoverable surface.
	for _, args := range [][]string{
		{"setup"}, {"module", "list"}, {"schema", "status"}, {"schema", "commit"}, {"schema", "log"}, {"schema", "edit", "x"},
		{"catalog"}, {"repo", "init"}, {"verify"}, {"validate", "--all"},
		{"model", "describe", "git-inventory"}, {"run-set", "git-inventory"},
	} {
		command := exec.Command(pudlBin, args...)
		command.Dir, command.Env = root, env
		output, err := command.CombinedOutput()
		require.Error(t, err, "obsolete command should fail: %v: %s", args, output)
	}
	require.NoError(t, json.Unmarshal(cli("init", "--global", "--json"), &initResult))
	require.Equal(t, "global", initResult["mode"])
	require.DirExists(t, global)
}
