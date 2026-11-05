package fileset

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const rulesFile = "schema/pudl/rules/demo.cue"

var testKind = Kind{Noun: "pack", RecordDir: ".packs"}

// demoFiles is a small file set in its current version.
func demoFiles() map[string][]byte {
	return map[string][]byte{
		rulesFile:                       []byte("package rules\n// current version: graph_edge\n"),
		"schema/models/demo.cue":        []byte("package models\n"),
		"populators/demo/baseline.json": []byte("[]\n"),
	}
}

func install(root string, opts Options) (*Result, error) {
	return Install(root, testKind, "demo", demoFiles(), opts)
}

func read(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	require.NoError(t, err)
	return string(data)
}

// pretendOldVersion makes the workspace look as if an earlier version of path
// was installed: different content, recorded (or not) in the manifest.
func pretendOldVersion(t *testing.T, root, path, old string, recorded bool) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(old), 0o644))
	m, err := readManifest(root, testKind, "demo")
	require.NoError(t, err)
	if recorded {
		m[path] = digest([]byte(old))
	} else {
		delete(m, path)
	}
	data, err := json.Marshal(map[string]any{"pack": "demo", "files": m})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath(root, testKind, "demo"), data, 0o644))
}

func TestInstallRecordsAManifestAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	res, err := install(root, Options{})
	require.NoError(t, err)
	require.NotEmpty(t, res.Added)
	require.Empty(t, res.Upgraded)
	m, err := readManifest(root, testKind, "demo")
	require.NoError(t, err)
	require.Len(t, m, len(res.Files))
	require.FileExists(t, filepath.Join(root, ".packs", "demo.json"), "recorded under the kind's record directory")

	again, err := install(root, Options{})
	require.NoError(t, err)
	require.Len(t, again.Unchanged, len(res.Files))
	require.Empty(t, again.Added)
}

func TestInstallUpgradesFilesTheUserDidNotEdit(t *testing.T) {
	root := t.TempDir()
	_, err := install(root, Options{})
	require.NoError(t, err)
	pretendOldVersion(t, root, rulesFile, "package rules\n// previous version\n", true)

	res, err := install(root, Options{})
	require.NoError(t, err)
	require.Equal(t, []string{rulesFile}, res.Upgraded)
	require.Contains(t, read(t, root, rulesFile), "graph_edge", "the new version is in place")
}

func TestInstallKeepsEditsAndUnrecordedFilesAsConflicts(t *testing.T) {
	for name, recorded := range map[string]bool{"edited since install": true, "installed without a record": false} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			_, err := install(root, Options{})
			require.NoError(t, err)
			pretendOldVersion(t, root, rulesFile, "package rules\n// previous version\n", recorded)
			if recorded {
				// The user edited the recorded version.
				require.NoError(t, os.WriteFile(filepath.Join(root, rulesFile), []byte("package rules\n// mine\n"), 0o644))
			}
			before := read(t, root, rulesFile)

			_, err = install(root, Options{})
			var conflict *ConflictError
			require.True(t, errors.As(err, &conflict), "%v", err)
			require.Equal(t, []string{rulesFile}, conflict.Paths)
			require.Contains(t, err.Error(), "pudl pack install demo --overwrite")
			require.Equal(t, before, read(t, root, rulesFile), "nothing is written on a conflict")

			res, err := install(root, Options{Overwrite: true})
			require.NoError(t, err)
			require.Equal(t, []string{rulesFile}, res.Replaced)
			require.Equal(t, rulesFile+".bak", res.Backups[rulesFile])
			require.Equal(t, before, read(t, root, rulesFile+".bak"), "the user's version is kept")
			require.Contains(t, read(t, root, rulesFile), "graph_edge")

			// A second overwrite does not clobber the first backup.
			require.NoError(t, os.WriteFile(filepath.Join(root, rulesFile), []byte("package rules\n// again\n"), 0o644))
			res, err = install(root, Options{Overwrite: true})
			require.NoError(t, err)
			require.Equal(t, rulesFile+".bak.1", res.Backups[rulesFile])
		})
	}
}

func TestRecordedState(t *testing.T) {
	root := t.TempDir()
	state, err := RecordedState(root, testKind, "demo", demoFiles())
	require.NoError(t, err)
	require.Equal(t, NotInstalled, state)

	_, err = install(root, Options{})
	require.NoError(t, err)
	state, err = RecordedState(root, testKind, "demo", demoFiles())
	require.NoError(t, err)
	require.Equal(t, Installed, state)

	pretendOldVersion(t, root, rulesFile, "package rules\n// previous version\n", true)
	state, err = RecordedState(root, testKind, "demo", demoFiles())
	require.NoError(t, err)
	require.Equal(t, Outdated, state)
}
