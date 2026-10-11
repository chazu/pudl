package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVendoredDependenciesHaveConsistentPriorityAndConfinement(t *testing.T) {
	root := repoWorkspace(t, t.TempDir(), "repo")
	pudl := filepath.Join(root, ".pudl")
	for _, name := range []string{"first", "second"} {
		require.NoError(t, os.MkdirAll(filepath.Join(pudl, "vendor", name, "schema"), 0755))
	}
	file := filepath.Join(pudl, "workspace.cue")
	require.NoError(t, os.WriteFile(file, []byte(`name:"repo"
dependencies:["vendor/first","vendor/second"]`), 0644))
	policy, err := Resolve(root, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(pudl, "schema"), filepath.Join(pudl, "vendor", "first", "schema"), filepath.Join(pudl, "vendor", "second", "schema")}, policy.SchemaSearchPaths)
	require.Equal(t, []string{filepath.Join(pudl, "vendor", "second", "schema", "pudl", "rules"), filepath.Join(pudl, "vendor", "first", "schema", "pudl", "rules"), filepath.Join(pudl, "schema", "pudl", "rules")}, policy.RuleSearchPaths)
	for _, path := range []string{"../elsewhere", "/absolute", "vendor/missing"} {
		require.NoError(t, os.WriteFile(file, []byte(`dependencies:["`+path+`"]`), 0644))
		_, err := Resolve(root, t.TempDir())
		require.Error(t, err)
	}
	require.NoError(t, os.Symlink(filepath.Join(pudl, "vendor", "first"), filepath.Join(pudl, "vendor", "linked")))
	require.NoError(t, os.WriteFile(file, []byte(`dependencies:["vendor/linked"]`), 0644))
	_, err = Resolve(root, t.TempDir())
	require.ErrorContains(t, err, "symlink")
}
