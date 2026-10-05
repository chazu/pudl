package acute

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Human and JSON output must state the same findings: every resource, reason
// and field difference in the JSON appears in the human rendering.
func TestWriteMarkdownStatesEveryJSONFinding(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	result := ModelDriftResult{
		Drifted: InventorySetDiff(
			[]map[string]any{
				{"_schema": "s", "name": "a", "port": int64(1), "mode": "on"},
				{"_schema": "s", "name": "gone"},
				{"_schema": "s", "state": "x"},
			},
			[]ObservedRecord{{Data: map[string]any{"_schema": "s", "name": "a", "port": "1", "mode": "off"}, ObservedAt: &at}},
			nil,
		),
		SnapshotID: "snap-1",
		ObservedAt: &at,
	}
	var human strings.Builder
	result.WriteMarkdown(&human)
	out := human.String()

	raw, err := json.Marshal(result)
	require.NoError(t, err)
	var decoded ModelDriftResult
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded.Drifted, 3)
	for _, d := range decoded.Drifted {
		assert.Contains(t, out, d.Resource+" ("+d.Reason+")")
		for _, f := range d.Fields {
			assert.Contains(t, out, f.Detail())
		}
	}
	assert.Contains(t, out, "- 3 drifted resource(s):")
	assert.Contains(t, out, `port: expected 1, observed "1"`)
	assert.Contains(t, out, `mode: expected "on", observed "off"`)
	assert.Contains(t, out, "- verified: false")
	assert.Contains(t, out, "- snapshot_id: snap-1")
	assert.Contains(t, out, "- observed_at: 2026-10-05T12:00:00Z")
}

func TestWriteMarkdownClean(t *testing.T) {
	var b strings.Builder
	ModelDriftResult{Clean: true, Verified: true, ObservationID: "obs-1"}.WriteMarkdown(&b)
	assert.Equal(t, "- ∅ (clean — all desired resources exist and match)\n- verified: true\n- observation_id: obs-1\n", b.String())
}

func TestStampObservedAtKeepsPerFindingTimes(t *testing.T) {
	own := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := ModelDriftResult{Drifted: []ResourceDrift{{Resource: "a"}, {Resource: "b", ObservedAt: &own}}}
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	r.StampObservedAt(now)
	assert.True(t, r.ObservedAt.Equal(now))
	assert.True(t, r.Drifted[0].ObservedAt.Equal(now))
	assert.True(t, r.Drifted[1].ObservedAt.Equal(own))
}
