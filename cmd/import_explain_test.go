package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/importer"
	"github.com/stretchr/testify/require"
)

func TestImportExplainPersistedAndExplicitFallback(t *testing.T) {
	root := consolidationWorkspace(t)
	resetImportFlags(t)
	beforeExplain := importExplain
	t.Cleanup(func() { importExplain = beforeExplain })
	importExplain = true
	for _, tc := range []struct {
		name, schema string
		fallback     bool
	}{
		{"inferred", "", false},
		{"fallback", "pudl/k8s.#Resource", true},
		{"unavailable", "mu/missing@v1#Resource", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.name+".json")
			require.NoError(t, os.WriteFile(path, []byte(`{"name":"`+tc.name+`"}`), 0600))
			require.NoError(t, importCmd.Flags().Set("path", path))
			importSchema = tc.schema
			output := captureQueryOutput(t, func() error { return runImportCommand(importCmd, nil) })
			var results []importOutcome
			require.NoError(t, json.Unmarshal([]byte(output), &results))
			require.Len(t, results, 1)
			result := results[0].ImportResult
			require.NotNil(t, result.Explanation)
			require.Equal(t, result.AssignedSchema, result.Explanation.Selected)
			// Inferred unknown records choose the catchall; manual fallback is explicit.
			if tc.schema != "" {
				require.Equal(t, tc.fallback, result.Explanation.Fallback)
			}
			raw, err := os.ReadFile(result.MetadataPath)
			require.NoError(t, err)
			var metadata importer.ImportMetadata
			require.NoError(t, json.Unmarshal(raw, &metadata))
			require.Equal(t, result.Explanation.Reason, metadata.SchemaInfo.AssignmentReason)
			require.NotNil(t, metadata.SchemaInfo.Explanation)
			if tc.fallback {
				require.NotEmpty(t, result.Explanation.Attempts[0].FailurePaths)
			}
			if tc.name == "unavailable" {
				require.Contains(t, result.Explanation.Reason, "unavailable")
			}
		})
	}
}

func TestImportExplanationRemainsOptIn(t *testing.T) {
	root := consolidationWorkspace(t)
	resetImportFlags(t)
	beforeExplain := importExplain
	importExplain = false
	t.Cleanup(func() { importExplain = beforeExplain })
	path := filepath.Join(root, "ordinary.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name":"ordinary"}`), 0600))
	require.NoError(t, importCmd.Flags().Set("path", path))
	output := captureQueryOutput(t, func() error { return runImportCommand(importCmd, nil) })
	var results []importOutcome
	require.NoError(t, json.Unmarshal([]byte(output), &results))
	require.Len(t, results, 1)
	require.Nil(t, results[0].Explanation)
	raw, err := os.ReadFile(results[0].MetadataPath)
	require.NoError(t, err)
	var metadata importer.ImportMetadata
	require.NoError(t, json.Unmarshal(raw, &metadata))
	require.NotEmpty(t, metadata.SchemaInfo.AssignmentReason)
	require.Nil(t, metadata.SchemaInfo.Explanation)
}
