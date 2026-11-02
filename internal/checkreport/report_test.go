package checkreport

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/systemmodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/acute"
)

// The human run report renders drift from the same struct the JSON report
// serializes, so both carry the same findings.
func TestRunReportDriftHumanMatchesJSON(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	report := &ModelReport{
		RunID: "run-1", Model: "m", Mode: "observe-only", OK: true,
		Drift: &acute.ModelDriftResult{
			Drifted: acute.InventorySetDiff(
				[]map[string]any{{"_schema": "s", "name": "a", "port": int64(1), "state": "on"}},
				[]acute.ObservedRecord{{Data: map[string]any{"_schema": "s", "name": "a", "port": "1", "state": "off"}, ObservedAt: &at}},
				nil,
			),
			SnapshotID: "snap-1",
			ObservedAt: &at,
		},
	}
	human, err := report.Render(false)
	require.NoError(t, err)
	machine, err := report.Render(true)
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

func TestRunReportMarkdownShowsFailureContext(t *testing.T) {
	text := (&ModelReport{Model: "pods", RunID: "run_1", Mode: "observe-only", OK: false, Error: "boom"}).Markdown()
	assert.True(t, strings.Contains(text, "status: FAILED"))
}

func TestRunReportMarkdownShowsObservedAndNewRecords(t *testing.T) {
	text := (&ModelReport{Model: "pods", Populate: &Populate{Records: 55, NewRecords: 0}}).Markdown()
	assert.Contains(t, text, "- records: 55 (0 new)")
}

func TestRenderRunPlan_ObserveOnly(t *testing.T) {
	m := &systemmodel.SystemModel{
		Name:     "fw",
		Populate: systemmodel.Populate{Runs: []systemmodel.CommandRun{{Argv: []string{"gcloud"}}}},
		Checks:   []systemmodel.Check{{Name: "open"}},
	}
	plan := RenderPlan(m)
	assert.Contains(t, plan, "model:    fw")
	assert.Contains(t, plan, "populate: command (1 runs)")
	assert.Contains(t, plan, "checks:   1")
	assert.Contains(t, plan, "mode:     observe-only")
}

func TestReportSummaryDistinguishesReplayAndUncertainChecks(t *testing.T) {
	r := &ModelReport{RunID: "r", CompletionStatus: "succeeded", OK: true, Drift: &acute.ModelDriftResult{Clean: false, Verified: false, Drifted: []acute.ResourceDrift{{Resource: "x"}}}}
	s := Summarize(r, nil)
	require.Equal(t, "succeeded", s.Execution)
	require.Equal(t, "drifted", s.Conformity)
	require.Equal(t, "recorded", s.Verification)
	r.Drift = &acute.ModelDriftResult{Clean: true, Verified: true}
	r.Checks = []CheckResult{{Outcome: "unknown", Severity: "warn"}}
	s = Summarize(r, nil)
	require.Equal(t, "not-verified", s.Verification)
	require.Equal(t, 1, s.UnknownChecks)
}
