package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func bundleCommandFlags(t *testing.T, command *cobra.Command, flags map[string]string) {
	t.Helper()
	for name, value := range flags {
		flag := command.Flags().Lookup(name)
		require.NotNil(t, flag, "missing bundle command flag %s", name)
		previous := flag.Value.String()
		require.NoError(t, command.Flags().Set(name, value))
		t.Cleanup(func() { _ = command.Flags().Set(name, previous) })
	}
}

func TestBundleCommandsRestoreMovedWorkspace(t *testing.T) {
	root := consolidationWorkspace(t)
	resetImportFlags(t)
	source := filepath.Join(t.TempDir(), "source.json")
	require.NoError(t, os.WriteFile(source, []byte(`{"name":"bundle-cli","serial":9007199254740993}`), 0600))
	require.NoError(t, importCmd.Flags().Set("path", source))
	output := captureQueryOutput(t, func() error { return runImportCommand(importCmd, nil) })
	var results []importOutcome
	require.NoError(t, json.Unmarshal([]byte(output), &results))
	require.Len(t, results, 1)
	archive := filepath.Join(t.TempDir(), "cli-backup.tar.zst")
	bundleCommandFlags(t, exportCmd, map[string]string{"bundle": archive, "id": "", "origin": "", "schema": "", "output": ""})
	require.NoError(t, runExportCommand(exportCmd, nil))
	_, err := os.Stat(archive)
	require.NoError(t, err)
	// Removing the source workspace makes stale absolute paths fail immediately.
	require.NoError(t, os.RemoveAll(root))
	target := t.TempDir()
	t.Chdir(target)
	bundleCommandFlags(t, initCmd, map[string]string{"from-bundle": archive, "global": "false", "force": "false"})
	restoredOutput := captureQueryOutput(t, func() error { return runInitCommand(initCmd, nil) })
	var initResult map[string]any
	require.NoError(t, json.Unmarshal([]byte(restoredOutput), &initResult))
	restoredRoot := filepath.Join(target, ".pudl")
	require.Equal(t, restoredRoot, initResult["path"])
	cfg, err := config.LoadFrom(restoredRoot)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(restoredRoot, "data"), cfg.DataPath)
	db, err := database.NewCatalogDB(restoredRoot)
	require.NoError(t, err)
	defer db.Close()
	entry, err := db.GetEntry(results[0].ID)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(restoredRoot, "data"), cfg.DataPath)
	raw, err := os.ReadFile(entry.StoredPath)
	require.NoError(t, err)
	require.Contains(t, string(raw), "9007199254740993")
	// init must not reinterpret restoration as repair or overwrite of an existing workspace.
	sentinel := filepath.Join(restoredRoot, "authored-sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0600))
	require.Error(t, runInitCommand(initCmd, nil))
	raw, err = os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(raw))
}

func TestBundleExportCommandFailurePreservesExistingDestination(t *testing.T) {
	root := consolidationWorkspace(t)
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	require.NoError(t, db.AddEntry(database.CatalogEntry{ID: "missing-evidence", StoredPath: filepath.Join(root, "data/raw/missing.json"), Format: "json", Schema: "pudl/core.#Item", Origin: "bundle-cli"}))
	require.NoError(t, db.Close())
	path := filepath.Join(t.TempDir(), "existing.tar.zst")
	require.NoError(t, os.WriteFile(path, []byte("existing destination"), 0600))
	bundleCommandFlags(t, exportCmd, map[string]string{"bundle": path, "id": "", "schema": "", "origin": "", "output": ""})
	require.Error(t, runExportCommand(exportCmd, nil))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "existing destination", string(data))
}
