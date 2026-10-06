package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrphanCleanupCannotFollowEscapingDirectories(t *testing.T) {
	for _, subtree := range []string{"raw", "metadata"} {
		for _, nested := range []bool{false, true} {
			t.Run(subtree+map[bool]string{false: "/subtree", true: "/nested"}[nested], func(t *testing.T) {
				workspace := t.TempDir()
				db, err := NewCatalogDB(workspace)
				require.NoError(t, err)
				defer db.Close()
				external := t.TempDir()
				victim := filepath.Join(external, "keep.json")
				require.NoError(t, os.WriteFile(victim, []byte("external evidence"), 0600))
				link := filepath.Join(workspace, "data", subtree)
				if nested {
					link = filepath.Join(link, "escape")
				}
				require.NoError(t, os.MkdirAll(filepath.Dir(link), 0755))
				require.NoError(t, os.Symlink(external, link))
				removed, err := db.RemoveCommittedOrphan(filepath.Join(link, "keep.json"))
				require.Error(t, err)
				require.False(t, removed)
				body, err := os.ReadFile(victim)
				require.NoError(t, err)
				require.Equal(t, "external evidence", string(body))
			})
		}
	}
}

func TestOrphanCleanupHonorsConfiguredDataDirectory(t *testing.T) {
	db, err := NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "raw", "keep.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte("keep"), 0600))
	removed, err := db.removeCommittedOrphanAt(path, dataDir)
	require.NoError(t, err)
	require.True(t, removed)
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}
