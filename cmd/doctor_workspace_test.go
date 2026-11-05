package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/repo"
)

// The global config.yaml records absolute schema_path and data_path. A copied
// global workspace (HOME=<copy> pudl --global doctor) must check the copy's
// catalog and schemas, as the other commands do, not the original's.
func TestDoctorChecksTheCopiedGlobalWorkspace(t *testing.T) {
	cliWorkspace(t)
	original := os.Getenv("HOME")
	// Lay out ~/.pudl offline (global init fetches CUE modules), then record
	// absolute paths in config.yaml as global init does.
	require.NoError(t, repo.Init(repo.InitOptions{Dir: original}))
	originalRoot := filepath.Join(original, ".pudl")
	require.NoError(t, config.DefaultConfigFor(originalRoot).SaveTo(originalRoot))
	importGlobal(t, `{"name":"shared"}`)

	copied := t.TempDir()
	require.NoError(t, os.CopyFS(filepath.Join(copied, ".pudl"), os.DirFS(originalRoot)))
	// The original diverges after the copy: it gains an entry the copy lacks.
	importGlobal(t, `{"name":"original-only"}`)

	t.Setenv("HOME", copied)
	r := runCLI(t, "doctor", "--global", "--json")
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), "stdout=%s stderr=%s", r.Stdout, r.Stderr)
	require.Empty(t, report.Error)
	assert.Len(t, report.Catalog, 1, "doctor must scan the copy's catalog, not the original's")

	copiedSchema := filepath.Join(copied, ".pudl", "schema")
	found := false
	for _, health := range report.Health {
		if health.Name == "Schema Loading" {
			found = true
			assert.Contains(t, health.Details, copiedSchema, "doctor must load the copy's schemas")
		}
		assert.NotContains(t, health.Details, original, "%s checked the original workspace", health.Name)
	}
	assert.True(t, found)
}

// importGlobal imports one JSON document into the global workspace.
func importGlobal(t *testing.T, document string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.json")
	writeFile(t, path, document)
	r := runCLI(t, "import", "--global", "--path", path)
	require.NoError(t, r.Err, "stdout=%s stderr=%s", r.Stdout, r.Stderr)
}
