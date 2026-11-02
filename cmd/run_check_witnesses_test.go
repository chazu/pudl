package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/stretchr/testify/require"
)

func TestCheckWitnessesDeterministicAndBounded(t *testing.T) {
	var rows []datalog.Tuple
	for i := 24; i >= 0; i-- {
		rows = append(rows, datalog.Tuple{Args: map[string]any{"name": "app", "value": fmt.Sprintf("%02d", i), "entry_id": fmt.Sprintf("entry%02d", i)}})
	}
	witnesses, truncated := checkWitnesses(rows, "empty")
	require.True(t, truncated)
	require.Len(t, witnesses, checkWitnessLimit)
	require.Equal(t, "entry00", witnesses[0].EvidenceIDs[0])
	reversed := append([]datalog.Tuple{}, rows...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	again, _ := checkWitnesses(reversed, "empty")
	require.Equal(t, witnesses, again)
}
func TestCheckWitnessesBoundValuesWithoutChangingOriginalEvidence(t *testing.T) {
	nested := map[string]any{"provider": "aws", "number": json.Number("9007199254740993")}
	rows := []datalog.Tuple{{Args: map[string]any{"nested": nested, "huge": strings.Repeat("x", 5000)}}}
	witnesses, _ := checkWitnesses(rows, "empty")
	encoded, err := json.Marshal(witnesses)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "9007199254740993")
	require.Contains(t, string(encoded), "omitted: value exceeds witness limit")
	require.Equal(t, "aws", nested["provider"])
}
func TestNonemptyFailureHasExplanationWithoutInventedWitness(t *testing.T) {
	witnesses, truncated := checkWitnesses(nil, "nonempty")
	require.Nil(t, witnesses)
	require.False(t, truncated)
	report := RunReport{Model: "m", Checks: []CheckResult{{Name: "evidence", Expect: "nonempty", Query: "observed", Scope: "run", Passed: false}}}
	human, err := report.render(false)
	require.NoError(t, err)
	require.Contains(t, human, "## findings")
	require.Contains(t, human, "no matching evidence for relation observed in run scope")
	jsonReport, err := report.render(true)
	require.NoError(t, err)
	require.Contains(t, jsonReport, `"checks"`)
}
