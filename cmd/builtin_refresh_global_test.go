package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/importer"
)

// pudl-8eh: a global workspace whose built-ins came from a pudl whose
// #SystemModel had no populate.empty_when. A pack model using the field failed
// to load in pudl check and pudl model validate until something ran an import.
func TestGlobalModelCommandsRefreshStaleSystemModel(t *testing.T) {
	schema := globalWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp", "--global")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	staleSystemModel(t, schema)

	r = runCLI(t, "--global", "model", "validate", "gcp-fixture-firewalls")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stderr, "Updated pudl's built-in schemas")
	data, err := os.ReadFile(filepath.Join(schema, "pudl", "systemmodel", "systemmodel.cue"))
	require.NoError(t, err)
	require.Contains(t, string(data), "empty_when")
}

// pudl init --global on an existing ~/.pudl repairs it like a local pudl init:
// a missing built-in comes back even when the stamp says the built-ins are
// current, and the configuration is kept.
func TestInitGlobalRepairsExistingWorkspace(t *testing.T) {
	schema := globalWorkspace(t)
	configPath := config.ConfigPath(filepath.Dir(schema))
	writeFile(t, configPath, "schema_path: "+schema+"\ndata_path: "+filepath.Join(filepath.Dir(schema), "data")+"\nversion: \"1.0\"\n# mine\n")
	systemModel := filepath.Join(schema, "pudl", "systemmodel", "systemmodel.cue")
	require.NoError(t, os.Remove(systemModel))

	r := runCLI(t, "init", "--global")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "configuration kept")
	require.FileExists(t, systemModel)
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "# mine", "the configuration is not rewritten")
}

// globalWorkspace initializes ~/.pudl in a fresh HOME without network access
// (pudl init --global fetches CUE modules) and runs from a directory outside
// any repository workspace. It returns the global schema path.
func globalWorkspace(t *testing.T) string {
	t.Helper()
	cliWorkspace(t)
	t.Chdir(t.TempDir())
	cfg := config.DefaultConfig()
	require.NoError(t, cfg.Save())
	writeFile(t, filepath.Join(cfg.SchemaPath, "cue.mod", "module.cue"), "module: \"pudl.schemas@v0\"\nlanguage: version: \"v0.16.0\"\n")
	require.NoError(t, importer.CopyBootstrapSchemas(cfg.SchemaPath))
	return cfg.SchemaPath
}

// staleSystemModel makes schemaPath look as if an older pudl installed its
// built-ins: no stamp, and a #SystemModel without populate.empty_when.
func staleSystemModel(t *testing.T, schemaPath string) {
	t.Helper()
	path := filepath.Join(schemaPath, "pudl", "systemmodel", "systemmodel.cue")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(string(data), "\n")
	var kept []string
	for _, line := range lines {
		if !strings.Contains(line, "empty_when?:") {
			kept = append(kept, line)
		}
	}
	require.Less(t, len(kept), len(lines), "the field was removed")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644))
	require.NoError(t, os.Remove(filepath.Join(schemaPath, "pudl", ".bootstrap-digest")))
}
