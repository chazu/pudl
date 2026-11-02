package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/repair"
)

// Doctor flags a payload written group- and world-readable before data was
// owner-only; --fix --dry-run leaves it, and --fix tightens it.
func TestDoctorTightensWorldReadableData(t *testing.T) {
	pudlDir := cliWorkspace(t)
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	payload := filepath.Join(pudlDir, "data", "raw", "2025", "01", "02", "legacy.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(payload), 0o755))
	require.NoError(t, os.WriteFile(payload, []byte(`{}`), 0o644))
	require.NoError(t, os.Chmod(payload, 0o644))

	flagged := func(r doctorRepair) bool {
		for _, c := range r.Changes {
			if c.Subject == payload && c.From == "0644" && c.To == "0600" {
				return true
			}
		}
		return false
	}
	mode := func(path string) os.FileMode {
		info, err := os.Stat(path)
		require.NoError(t, err)
		return info.Mode().Perm()
	}

	_, repairs := doctorRepairs(t)
	require.Equal(t, repair.StatusNeeded, repairs["data-permissions"].Status)
	require.True(t, flagged(repairs["data-permissions"]), "%+v", repairs["data-permissions"])
	require.Contains(t, repairs["data-permissions"].Apply, "data-permissions")

	_, repairs = doctorRepairs(t, "--fix", "--dry-run", "--only", "data-permissions")
	require.True(t, flagged(repairs["data-permissions"]))
	require.Equal(t, os.FileMode(0o644), mode(payload))

	_, repairs = doctorRepairs(t, "--fix", "--only", "data-permissions")
	require.Equal(t, repair.StatusApplied, repairs["data-permissions"].Status, "%+v", repairs["data-permissions"])
	require.Equal(t, os.FileMode(0o600), mode(payload))
	require.Equal(t, os.FileMode(0o700), mode(filepath.Dir(payload)))

	_, repairs = doctorRepairs(t)
	require.Equal(t, repair.StatusOK, repairs["data-permissions"].Status, "%+v", repairs["data-permissions"])
}
