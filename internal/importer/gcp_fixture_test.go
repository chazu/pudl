package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/test/testutil"
	"github.com/stretchr/testify/require"
)

// gcpFixture stages a workspace whose schema repository holds the given CUE
// source as package pudl/gcp, opens an importer over it, and writes the named
// source file.
func gcpFixture(t *testing.T, schemaSource, name, content string) (*EnhancedImporter, string) {
	t.Helper()
	setup := testutil.NewTempDirSetup(t)
	workspace := setup.CreatePUDLWorkspace()
	setup.AddBootstrapSchemas(workspace)
	dir := filepath.Join(workspace.SchemaDir, "pudl", "gcp")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gcp.cue"), []byte(schemaSource), 0o644))
	source := setup.WriteFile(name, content)
	imp, err := NewEnhancedImporter(workspace.DataDir, workspace.SchemaDir, workspace.Root)
	require.NoError(t, err)
	t.Cleanup(func() { _ = imp.Close() })
	return imp, source
}

const cloudRunSchema = `package gcp

#CloudRunService: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.run.service"
		identity_fields: ["project", "metadata.name", "metadata.labels.\"cloud.googleapis.com/location\""]
	}
	kind: "Service"
	...
}
`

func TestImportReportsAndResolvesQuotedIdentity(t *testing.T) {
	records := `{"kind":"Service","project":"p","metadata":{"name":"a","labels":{"cloud.googleapis.com/location":"us-east1"}}}
{"kind":"Service","project":"p","metadata":{"name":"b"}}
`
	imp, source := gcpFixture(t, cloudRunSchema, "run.ndjson", records)
	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#CloudRunService"})
	require.NoError(t, err)
	require.Equal(t, 1, result.IdentityUnresolved, "second record lacks the location label")
	require.Contains(t, result.IdentityError, "cloud.googleapis.com/location")

	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	var resolved int
	for _, item := range items {
		if item.IdentityJSON != nil {
			resolved++
			require.Contains(t, *item.IdentityJSON, "us-east1")
		}
	}
	require.Equal(t, 1, resolved)
}

// writeSibling writes another source file next to an existing one.
func writeSibling(t *testing.T, existing, name, content string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(existing), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
