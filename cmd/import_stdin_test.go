package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
)

// withStdin replaces os.Stdin with a pipe carrying content for the duration
// of the test.
func withStdin(t *testing.T, content string) {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	_, err = writer.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = previous
		reader.Close()
	})
}

func resetImportFlags(t *testing.T) {
	t.Helper()
	require.NoError(t, importCmd.Flags().Set("path", ""))
	previousOrigin, previousSchema, previousFormat := importOrigin, importSchema, importFormat
	importOrigin, importSchema, importFormat = "", "", ""
	t.Cleanup(func() {
		importOrigin, importSchema, importFormat = previousOrigin, previousSchema, previousFormat
		_ = importCmd.Flags().Set("path", "")
	})
}

func TestImportPathIsNotRequired(t *testing.T) {
	flag := importCmd.Flags().Lookup("path")
	require.NotNil(t, flag)
	_, required := flag.Annotations[cobra.BashCompOneRequiredFlag]
	assert.False(t, required, "a required --path makes piped stdin import unreachable")
}

func TestImportFromPipedStdin(t *testing.T) {
	root := consolidationWorkspace(t)
	resetImportFlags(t)
	withStdin(t, `{"kind":"Widget","name":"from-stdin"}`)

	require.NoError(t, runImportCommand(importCmd, nil))

	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	result, err := db.QueryEntries(database.FilterOptions{Origin: "stdin"}, database.QueryOptions{})
	require.NoError(t, err)
	assert.Len(t, result.Entries, 1)

	leftovers, _ := filepath.Glob(filepath.Join(os.TempDir(), "stdin.json"))
	assert.Empty(t, leftovers, "stdin no longer stages under a fixed shared name")
}
