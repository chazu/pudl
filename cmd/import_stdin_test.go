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

func TestImportEnvelopeFromReadOnlyDirectoryKeepsOriginalOrigin(t *testing.T) {
	root := consolidationWorkspace(t)
	resetImportFlags(t)

	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "inventory.json")
	require.NoError(t, os.WriteFile(source,
		[]byte(`{"schema":{"module":"mu/test","version":"v1"},"data":{"name":"x"}}`), 0o644))
	require.NoError(t, os.Chmod(sourceDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(sourceDir, 0o755) })

	session, err := newImportSession()
	require.NoError(t, err)
	defer session.Close()

	result, err := importOneWithEnvelope(session.imp, session.options(source, ""))
	require.NoError(t, err)
	assert.Equal(t, "inventory", result.DetectedOrigin, "origin comes from the user's file, not the temp payload")
	assert.Equal(t, source, result.SourcePath)

	entries, err := os.ReadDir(sourceDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "nothing is written beside the source")
	leftovers, _ := filepath.Glob(filepath.Join(root, "data", "tmp", ".envelope-*"))
	assert.Empty(t, leftovers, "the payload temp file is removed")
}
