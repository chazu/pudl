package cmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/acute"
)

// The human run report renders drift from the same struct the JSON report
// serializes, so both carry the same findings.
func TestRunReportDriftHumanMatchesJSON(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	report := &RunReport{
		RunID: "run-1", Model: "m", Mode: "observe-only", OK: true,
		Drift: &ModelDriftResult{
			Drifted: acute.InventorySetDiff(
				[]map[string]any{{"_schema": "s", "name": "a", "port": int64(1), "state": "on"}},
				[]acute.ObservedRecord{{Data: map[string]any{"_schema": "s", "name": "a", "port": "1", "state": "off"}, ObservedAt: &at}},
				nil,
			),
			SnapshotID: "snap-1",
			ObservedAt: &at,
		},
	}
	human, err := report.render(false)
	require.NoError(t, err)
	machine, err := report.render(true)
	require.NoError(t, err)

	var decoded struct {
		Drift struct {
			Drifted []struct {
				Resource string            `json:"resource"`
				Reason   string            `json:"reason"`
				Diff     string            `json:"diff"`
				Fields   []acute.FieldDiff `json:"fields"`
			} `json:"drifted"`
			Verified   bool   `json:"verified"`
			SnapshotID string `json:"snapshot_id"`
			ObservedAt string `json:"observed_at"`
		} `json:"drift"`
	}
	require.NoError(t, json.Unmarshal([]byte(machine), &decoded))
	require.Len(t, decoded.Drift.Drifted, 1)
	finding := decoded.Drift.Drifted[0]
	require.Len(t, finding.Fields, 2)
	assert.Contains(t, human, finding.Resource+" ("+finding.Reason+"): "+finding.Diff)
	for _, f := range finding.Fields {
		assert.Contains(t, human, f.Detail())
	}
	assert.Contains(t, human, "- snapshot_id: "+decoded.Drift.SnapshotID)
	assert.Contains(t, human, "- observed_at: "+decoded.Drift.ObservedAt)
	assert.Contains(t, human, "- verified: false")
}
