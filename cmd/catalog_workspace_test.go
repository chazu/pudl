package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/repo"
)

// list, show, delete and import in a copied global workspace
// (HOME=<copy> pudl --global ...) must use the copy's catalog and data
// directory, not the original's absolute paths recorded in config.yaml.
func TestCatalogCommandsUseTheCopiedGlobalWorkspace(t *testing.T) {
	cliWorkspace(t)
	original := os.Getenv("HOME")
	// Lay out ~/.pudl offline (global init fetches CUE modules), then record
	// absolute paths in config.yaml as global init does.
	require.NoError(t, repo.Init(repo.InitOptions{Dir: original}))
	originalRoot := filepath.Join(original, ".pudl")
	require.NoError(t, config.DefaultConfigFor(originalRoot).SaveTo(originalRoot))
	importGlobal(t, `{"name":"shared"}`)

	copied := t.TempDir()
	copiedRoot := filepath.Join(copied, ".pudl")
	require.NoError(t, os.CopyFS(copiedRoot, os.DirFS(originalRoot)))
	// The original diverges after the copy: it gains an entry the copy lacks.
	importGlobal(t, `{"name":"original-only"}`)
	originalIDs := catalogIDs(t, originalRoot)
	require.Len(t, originalIDs, 2)
	shared := catalogIDs(t, copiedRoot)
	require.Len(t, shared, 1)
	var originalOnly string
	for id := range originalIDs {
		if !shared[id] {
			originalOnly = id
		}
	}
	sharedID := firstID(shared)

	t.Setenv("HOME", copied)

	r := runCLI(t, "list", "--global", "--json")
	require.NoError(t, r.Err, "stderr=%s", r.Stderr)
	assert.Contains(t, r.Stdout, idgen.HashToProquint(sharedID))
	assert.NotContains(t, r.Stdout, idgen.HashToProquint(originalOnly), "list read the original's catalog")

	r = runCLI(t, "show", "--global", idgen.HashToProquint(originalOnly))
	assert.Error(t, r.Err, "show found an entry only the original's catalog holds")

	path := filepath.Join(t.TempDir(), "doc.json")
	writeFile(t, path, `{"name":"copy-only"}`)
	r = runCLI(t, "import", "--global", "--path", path)
	require.NoError(t, r.Err, "stdout=%s stderr=%s", r.Stdout, r.Stderr)
	assert.Len(t, catalogIDs(t, originalRoot), 2, "import wrote to the original's catalog")
	copiedIDs := catalogIDs(t, copiedRoot)
	require.Len(t, copiedIDs, 2)
	for id := range copiedIDs {
		if id == sharedID {
			continue
		}
		entry := catalogEntry(t, copiedRoot, id)
		assert.True(t, strings.HasPrefix(entry.StoredPath, config.DataDir(copiedRoot)+string(filepath.Separator)),
			"import stored the payload outside the copy: %s", entry.StoredPath)
	}

	r = runCLI(t, "delete", "--global", "--force", idgen.HashToProquint(sharedID))
	require.NoError(t, r.Err, "stdout=%s stderr=%s", r.Stdout, r.Stderr)
	assert.False(t, catalogIDs(t, copiedRoot)[sharedID], "delete left the copy's entry")
	assert.True(t, catalogIDs(t, originalRoot)[sharedID], "delete removed the original's entry")
}

// catalogIDs returns the IDs of every entry in the catalog beneath pudlDir.
func catalogIDs(t *testing.T, pudlDir string) map[string]bool {
	t.Helper()
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, entry := range result.Entries {
		ids[entry.ID] = true
	}
	return ids
}

// catalogEntry returns one entry of the catalog beneath pudlDir.
func catalogEntry(t *testing.T, pudlDir, id string) *database.CatalogEntry {
	t.Helper()
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	entry, err := db.GetEntry(id)
	require.NoError(t, err)
	return entry
}

func firstID(ids map[string]bool) string {
	for id := range ids {
		return id
	}
	return ""
}
