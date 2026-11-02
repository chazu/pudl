package examples

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const rulesFile = "schema/pudl/rules/gcp_graph.cue"

func read(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	require.NoError(t, err)
	return string(data)
}

// pretendOldVersion makes the workspace look as if an earlier pack version of
// path was installed: different content, recorded (or not) in the manifest.
func pretendOldVersion(t *testing.T, root, path, old string, recorded bool) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(old), 0o644))
	m, err := readManifest(root, "gcp")
	require.NoError(t, err)
	if recorded {
		m[path] = digest([]byte(old))
	} else {
		delete(m, path)
	}
	data, err := json.Marshal(map[string]any{"example": "gcp", "files": m})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath(root, "gcp"), data, 0o644))
}

func TestInstallRecordsAManifestAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	res, err := Install(root, "gcp", Options{})
	require.NoError(t, err)
	require.NotEmpty(t, res.Added)
	require.Empty(t, res.Upgraded)
	m, err := readManifest(root, "gcp")
	require.NoError(t, err)
	require.Len(t, m, len(res.Files))

	again, err := Install(root, "gcp", Options{})
	require.NoError(t, err)
	require.Len(t, again.Unchanged, len(res.Files))
	require.Empty(t, again.Added)
}

func TestInstallUpgradesFilesTheUserDidNotEdit(t *testing.T) {
	root := t.TempDir()
	_, err := Install(root, "gcp", Options{})
	require.NoError(t, err)
	pretendOldVersion(t, root, rulesFile, "package rules\n// previous version\n", true)

	res, err := Install(root, "gcp", Options{})
	require.NoError(t, err)
	require.Equal(t, []string{rulesFile}, res.Upgraded)
	require.Contains(t, read(t, root, rulesFile), "graph_edge", "the new version is in place")
}

func TestInstallKeepsEditsAndLegacyFilesAsConflicts(t *testing.T) {
	for name, recorded := range map[string]bool{"edited since install": true, "installed by an older pudl": false} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			_, err := Install(root, "gcp", Options{})
			require.NoError(t, err)
			pretendOldVersion(t, root, rulesFile, "package rules\n// previous version\n", recorded)
			if recorded {
				// The user edited the recorded version.
				require.NoError(t, os.WriteFile(filepath.Join(root, rulesFile), []byte("package rules\n// mine\n"), 0o644))
			}
			before := read(t, root, rulesFile)

			_, err = Install(root, "gcp", Options{})
			var conflict *ConflictError
			require.True(t, errors.As(err, &conflict), "%v", err)
			require.Equal(t, []string{rulesFile}, conflict.Paths)
			require.Contains(t, err.Error(), "--overwrite")
			require.Equal(t, before, read(t, root, rulesFile), "nothing is written on a conflict")

			res, err := Install(root, "gcp", Options{Overwrite: true})
			require.NoError(t, err)
			require.Equal(t, []string{rulesFile}, res.Replaced)
			require.Equal(t, rulesFile+".bak", res.Backups[rulesFile])
			require.Equal(t, before, read(t, root, rulesFile+".bak"), "the user's version is kept")
			require.Contains(t, read(t, root, rulesFile), "graph_edge")

			// A second overwrite does not clobber the first backup.
			require.NoError(t, os.WriteFile(filepath.Join(root, rulesFile), []byte("package rules\n// again\n"), 0o644))
			res, err = Install(root, "gcp", Options{Overwrite: true})
			require.NoError(t, err)
			require.Equal(t, rulesFile+".bak.1", res.Backups[rulesFile])
		})
	}
}
