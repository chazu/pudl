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

func TestSmoke_ExampleInstall(t *testing.T) {
	root, _, env := exampleWorkspace(t, "git")
	cli := func(args ...string) string {
		t.Helper()
		command := exec.Command(pudlBin, args...)
		command.Dir, command.Env = root, env
		out, err := command.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return string(out)
	}
	cli("repo", "init")
	cli("example", "install", "git-inventory")
	require.Contains(t, cli("model", "validate", "git-inventory"), "is valid")
	var installed struct{ Files []string }
	require.NoError(t, json.Unmarshal([]byte(cli("example", "install", "git-inventory", "--json")), &installed))
	require.NotEmpty(t, installed.Files)
	model := filepath.Join(root, ".pudl/schema/models/git_inventory.cue")
	original, err := os.ReadFile(model)
	require.NoError(t, err)
	edited := append(original, []byte("\n// User customization.\n")...)
	require.NoError(t, os.WriteFile(model, edited, 0o644))
	// A conflict must not repair other files first or overwrite the user's edit.
	missing := filepath.Join(root, ".pudl/populators/git-inventory/baseline.json")
	require.NoError(t, os.Remove(missing))
	command := exec.Command(pudlBin, "example", "install", "git-inventory")
	command.Dir, command.Env = root, env
	out, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(out), "preserving your file")
	after, err := os.ReadFile(model)
	require.NoError(t, err)
	require.Equal(t, edited, after)
	require.NoFileExists(t, missing)
}

func TestSmoke_ExampleInstallRequiresWorkspace(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	command := exec.Command(pudlBin, "example", "install", "git-inventory")
	command.Dir = root
	command.Env = envWith(map[string]string{"HOME": home})
	out, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(out), "run pudl repo init first")
	require.NoDirExists(t, filepath.Join(root, ".pudl"))
	require.NoDirExists(t, filepath.Join(home, ".pudl"))
}

// The tutorial's PATH contains the installed CLI and only the requested tools.
// There is no source checkout to copy fixtures from and no inherited mu/python.
func exampleWorkspace(t *testing.T, tools ...string) (root, sandbox string, env []string) {
	t.Helper()
	repo, err := repoRoot()
	require.NoError(t, err)
	base := filepath.Join(repo, ".pudl/data/git-walkthrough/test-runs")
	require.NoError(t, os.MkdirAll(base, 0o755))
	sandbox, err = os.MkdirTemp(base, "example-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(sandbox) })
	root = filepath.Join(sandbox, "new user repo")
	bin := filepath.Join(sandbox, "installed tools")
	tmp := filepath.Join(sandbox, "tmp")
	for _, dir := range []string{root, bin, tmp} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	require.NoError(t, os.Symlink(pudlBin, filepath.Join(bin, "pudl")))
	for _, tool := range tools {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("required tool %s: %v", tool, err)
		}
		require.NoError(t, os.Symlink(path, filepath.Join(bin, tool)))
	}
	env = envWith(map[string]string{"PATH": bin, "HOME": filepath.Join(sandbox, "home"), "TMPDIR": tmp})
	return root, sandbox, env
}
