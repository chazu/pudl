package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
)

func TestReportCommandReadsStoredRunSetReportByID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	db, err := database.NewCatalogDB(config.GetPudlDir())
	require.NoError(t, err)
	payload, err := json.Marshal(acute.RunSetReport{
		ReportVersion: 1, RunSetID: "set_1", Mode: "observe-only",
		Status: database.RunStatusSucceeded,
	})
	require.NoError(t, err)
	require.NoError(t, db.SaveRunSetReport("set_1", payload))
	require.NoError(t, db.Close())

	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previousJSON })
	require.NoError(t, reportCmd.RunE(reportCmd, []string{"set_1"}))
	require.ErrorContains(t, reportCmd.RunE(reportCmd, []string{"missing"}), "not found")
}
