package packs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/fileset"
)

func TestInstallRecordsUnderPacks(t *testing.T) {
	root := t.TempDir()
	res, err := Install(root, "gcp", fileset.Options{})
	require.NoError(t, err)
	require.NotEmpty(t, res.Added)
	require.FileExists(t, filepath.Join(root, ".packs", "gcp.json"))
	require.NoDirExists(t, filepath.Join(root, ".examples"))

	state, err := State(root, "gcp")
	require.NoError(t, err)
	require.Equal(t, fileset.Installed, state)
}

// A pack installed by `pudl example install gcp` (before packs had their own
// command) left its record in .examples. That record is not read: the files
// are unrecorded, so changed ones are conflicts until --overwrite.
func TestExampleInstallRecordIsNotMigrated(t *testing.T) {
	root := t.TempDir()
	_, err := Install(root, "gcp", fileset.Options{})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".examples"), 0o755))
	require.NoError(t, os.Rename(filepath.Join(root, ".packs", "gcp.json"), filepath.Join(root, ".examples", "gcp.json")))

	state, err := State(root, "gcp")
	require.NoError(t, err)
	require.Equal(t, fileset.NotInstalled, state)

	const rules = "schema/pudl/rules/gcp_graph.cue"
	require.NoError(t, os.WriteFile(filepath.Join(root, rules), []byte("package rules\n// older pack\n"), 0o644))
	_, err = Install(root, "gcp", fileset.Options{})
	var conflict *fileset.ConflictError
	require.True(t, errors.As(err, &conflict), "%v", err)
	require.Equal(t, []string{rules}, conflict.Paths)
	require.Contains(t, err.Error(), "pudl pack install gcp --overwrite")

	_, err = Install(root, "gcp", fileset.Options{Overwrite: true})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, ".packs", "gcp.json"))
}

func TestUnknownPack(t *testing.T) {
	_, err := Install(t.TempDir(), "git-inventory", fileset.Options{})
	require.ErrorContains(t, err, `unknown pack "git-inventory" (available: gcp, aws, pulumi)`)
}
