package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/pkg/factstore"
	"github.com/stretchr/testify/require"
)

func TestProjectDefinitionsIgnoreGlobalAndLoadExplicitVendor(t *testing.T) {
	pudlDir := cliWorkspace(t)
	global := filepath.Join(os.Getenv("HOME"), ".pudl", "schema")
	writeFile(t, filepath.Join(global, "cue.mod", "module.cue"), "module: \"global.schemas@v0\"\nlanguage: version: \"v0.16.0\"\n")
	writeFile(t, filepath.Join(global, "custom", "thing.cue"), `package custom
#Thing: {_pudl:{schema_type:"base",resource_type:"thing",identity_fields:["name"]},name:string,...}`)
	r := runCLI(t, "schema", "list", "--json")
	require.NoError(t, r.Err)
	require.NotContains(t, r.Stdout, "custom.#Thing")
	r = runCLI(t, "schema", "list", "--global", "--json")
	require.NoError(t, r.Err)
	require.Contains(t, r.Stdout, "custom.#Thing")
	r = runCLI(t, "config", "--legacy-dependencies", "--json")
	require.NoError(t, r.Err)
	require.Contains(t, r.Stdout, "custom.#Thing")
	dep := filepath.Join(pudlDir, "vendor", "shared", "schema")
	writeFile(t, filepath.Join(dep, "cue.mod", "module.cue"), "module: \"shared.schemas@v0\"\nlanguage: version: \"v0.16.0\"\n")
	writeFile(t, filepath.Join(dep, "custom", "thing.cue"), `package custom
#Thing: {_pudl:{schema_type:"base",resource_type:"thing",identity_fields:["name"]},name:string,...}`)
	writeFile(t, filepath.Join(pudlDir, "workspace.cue"), `name:"test"
dependencies:["vendor/shared"]`)
	reloadSchemas()
	r = runCLI(t, "schema", "list", "--json")
	require.NoError(t, r.Err)
	require.Contains(t, r.Stdout, "custom.#Thing")
	before := r.Stdout
	// A different home cannot alter project classification or definitions.
	t.Setenv("HOME", t.TempDir())
	reloadSchemas()
	r = runCLI(t, "schema", "list", "--json")
	require.NoError(t, r.Err)
	require.JSONEq(t, before, r.Stdout)
	ws, err := factstore.DiscoverWorkspace(filepath.Dir(pudlDir))
	require.NoError(t, err)
	require.Equal(t, wsPolicy.RuleSearchPaths, ws.RulePaths)
	r = runCLI(t, "config", "--paths", "--json")
	require.NoError(t, r.Err)
	require.Contains(t, r.Stdout, "vendor/shared/schema")
}
