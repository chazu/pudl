package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoctorPayloadFlagReportsDamageWithoutRepair(t *testing.T) {
	root := consolidationWorkspace(t)
	resetImportFlags(t)
	source := filepath.Join(t.TempDir(), "evidence.json")
	require.NoError(t, os.WriteFile(source, []byte(`{"name":"doctor-exact","serial":9007199254740993}`), 0600))
	require.NoError(t, importCmd.Flags().Set("path", source))
	imported := captureQueryOutput(t, func() error { return runImportCommand(importCmd, nil) })
	var outcomes []importOutcome
	require.NoError(t, json.Unmarshal([]byte(imported), &outcomes))
	require.Len(t, outcomes, 1)
	bundleCommandFlags(t, doctorCmd, map[string]string{"health-only": "true", "verify-payloads": "true", "entry": ""})
	output := captureQueryOutput(t, func() error { return runDoctorCommand(doctorCmd, nil) })
	var healthy doctorReport
	require.NoError(t, json.Unmarshal([]byte(output), &healthy))
	found := false
	for _, diagnostic := range healthy.Health {
		if diagnostic.Name == "Payload Hashes" {
			found = true
			require.Equal(t, "ok", diagnostic.Status)
		}
	}
	require.True(t, found)
	path := outcomes[0].StoredPath
	damaged := []byte(`{"name":"doctor-exact","serial":9007199254740999}`)
	require.NoError(t, os.WriteFile(path, damaged, 0600))
	var returned error
	output = captureQueryOutput(t, func() error { returned = runDoctorCommand(doctorCmd, nil); return nil })
	require.Error(t, returned)
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(output), &report))
	require.False(t, report.OK)
	found = false
	for _, diagnostic := range report.Health {
		if diagnostic.Name == "Payload Hashes" {
			found = true
			require.Equal(t, "error", diagnostic.Status)
			require.Contains(t, diagnostic.Details, "hash mismatch")
			require.Contains(t, diagnostic.Details, outcomes[0].ID)
		}
	}
	require.True(t, found)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, damaged, raw)
	// Metadata and root configuration remain available; verification is read-only.
	_, err = os.Stat(filepath.Join(root, "config.yaml"))
	require.NoError(t, err)
}
