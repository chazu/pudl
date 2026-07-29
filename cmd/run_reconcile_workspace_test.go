package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetupReconcileWorkspaceUsesAbsoluteDesiredSources(t *testing.T) {
	cat, muRoot := acceptanceFixture(t)
	workspace, err := setupReconcileWorkspace(cat, &scriptedMu{}, convergentModel(), muRoot, t.TempDir(), "run_sources", false)
	require.NoError(t, err)
	t.Cleanup(workspace.Cleanup)

	dirs, err := filepath.Glob(filepath.Join(muRoot, workspacePrefix+"*"))
	require.NoError(t, err)
	require.Len(t, dirs, 1)

	config, err := os.ReadFile(filepath.Join(dirs[0], "mu.cue"))
	require.NoError(t, err)
	require.Contains(t, string(config), filepath.Join(dirs[0], "desired_0.json"))
}

func TestPersistConvergeErrorIncludesErrorDetails(t *testing.T) {
	cat := newRunCatalog(t.TempDir())
	defer cat.Close()

	report := &RunReport{
		RunID: "run_converge_error",
		Model: "m",
		Mode:  "converge",
		OK:    true,
	}
	applyRunError(report, errors.New("desired manifest could not be observed"))
	require.True(t, persistRunReport(cat, report, false))

	db, err := cat.required()
	require.NoError(t, err)
	stored, err := db.GetRunReport("run_converge_error")
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Contains(t, string(stored.Report), "desired manifest could not be observed")
}
