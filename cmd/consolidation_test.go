package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/repo"
	"github.com/chazu/pudl/internal/validator"
	"github.com/chazu/pudl/internal/workspace"
	"github.com/stretchr/testify/require"
)

func consolidationWorkspace(t *testing.T) string {
	t.Helper()
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(root)
	require.NoError(t, repo.Init(repo.InitOptions{Dir: root}))
	previous, previousJSON := wsPolicy, jsonOutput
	var err error
	wsPolicy, err = workspace.Resolve(root, config.GetPudlDir())
	require.NoError(t, err)
	jsonOutput = true
	t.Cleanup(func() {
		wsPolicy, jsonOutput = previous, previousJSON
		inference.ResetShared()
		validator.ResetSharedLoaders()
	})
	return filepath.Join(root, ".pudl")
}

func TestConsolidatedReportsSelectBothKindsAndLatest(t *testing.T) {
	root := consolidationWorkspace(t)
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	single, err := json.Marshal(RunReport{RunID: "single", Model: "demo", OK: true})
	require.NoError(t, err)
	set, err := json.Marshal(acute.RunSetReport{RunSetID: "arbitrary-id", Status: "succeeded"})
	require.NoError(t, err)
	require.NoError(t, db.SaveRunReport("single", "demo", single))
	require.NoError(t, db.SaveRunSetReport("arbitrary-id", set))
	require.NoError(t, func() error {
		_, err := db.DB().Exec("UPDATE run_reports SET created_at = ?", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
		return err
	}())
	for _, args := range [][]string{nil, {"arbitrary-id"}, {"single"}} {
		output := captureQueryOutput(t, func() error { return runReportCmd.RunE(runReportCmd, args) })
		expected := set
		if len(args) > 0 && args[0] == "single" {
			expected = single
		}
		require.JSONEq(t, string(expected), output)
	}
	_, err = db.DB().Exec("UPDATE run_reports SET created_at = ?", time.Now().UTC().Add(time.Hour))
	require.NoError(t, err)
	output := captureQueryOutput(t, func() error { return runReportCmd.RunE(runReportCmd, nil) })
	require.JSONEq(t, string(single), output, "latest must also select a newer standalone report")
	isSet, err := isRunSetOperation("arbitrary-id")
	require.NoError(t, err)
	require.True(t, isSet, "routing must use stored operation identity, not a prefix")
	require.NoError(t, db.SaveRunReport("arbitrary-id", "collision", single))
	require.ErrorContains(t, runReportCmd.RunE(runReportCmd, []string{"arbitrary-id"}), "ambiguous")
	_, err = isRunSetOperation("arbitrary-id")
	require.ErrorContains(t, err, "ambiguous")
}

func TestDoctorEmptyWorkspaceDoesNotCreateCatalog(t *testing.T) {
	root := consolidationWorkspace(t)
	path := filepath.Join(root, "data", "sqlite", "catalog.db")
	output := captureQueryOutput(t, func() error { return runDoctorCommand(doctorCmd, nil) })
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(output), &report))
	require.True(t, report.OK)
	require.Empty(t, report.Catalog)
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "diagnostics must not create an empty catalog")
}

func TestDoctorValidatesExplicitRecordsAndChecksInferredRecordsWithoutReclassification(t *testing.T) {
	root := consolidationWorkspace(t)
	dir := filepath.Join(root, "schema", "user", "demo")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "types.cue"), []byte(`package demo
#Thing: {
 _pudl: {schema_type: "base", resource_type: "test.thing", identity_fields: ["custom_field"]}
 custom_field: int
}
`), 0644))
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	path := filepath.Join(root, "data", "fixture.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"custom_field":42}`), 0644))
	explicit := "observe"
	for _, entry := range []database.CatalogEntry{
		{ID: "1111111100000000", StoredPath: path, Schema: "pudl/core.#Item", Format: "json", Origin: "custom", ImportTimestamp: time.Now()},
		{ID: "2222222200000000", StoredPath: path, Schema: "pudl/core.#Item", Format: "json", EntryType: &explicit, Origin: "custom", ImportTimestamp: time.Now()},
	} {
		require.NoError(t, db.AddEntry(entry))
	}
	previousEntry := doctorEntry
	doctorEntry = "1111111100000000"
	t.Cleanup(func() { doctorEntry = previousEntry })
	var report doctorReport
	var checkErr error
	output := captureQueryOutput(t, func() error { checkErr = runDoctorCommand(doctorCmd, nil); return nil })
	require.Error(t, checkErr)
	require.NoError(t, json.Unmarshal([]byte(output), &report))
	require.False(t, report.OK)
	require.Len(t, report.Catalog, 1)
	require.Equal(t, "mismatch", report.Catalog[0].Status)
	require.Equal(t, "user/demo.#Thing", report.Catalog[0].InferredSchema)
	stored, err := db.GetEntry(doctorEntry)
	require.NoError(t, err)
	require.Equal(t, "pudl/core.#Item", stored.Schema)
	doctorEntry = "2222222200000000"
	output = captureQueryOutput(t, func() error { return runDoctorCommand(doctorCmd, nil) })
	require.NoError(t, json.Unmarshal([]byte(output), &report))
	require.True(t, report.OK)
	require.False(t, report.Catalog[0].InferenceChecked)
	// Listing the active local catalog must include custom origins by default.
	output = captureQueryOutput(t, func() error { return runListCommand(listCmd, nil) })
	require.Contains(t, output, "custom")
}
