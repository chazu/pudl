package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/stretchr/testify/require"
)

func TestCheckWitnessesDeterministicBoundedAndScoped(t *testing.T) {
	model, scope := checkScopeFixture(t, "app")
	var rows []datalog.Tuple
	for i := 24; i >= 0; i-- {
		rows = append(rows, datalog.Tuple{Args: map[string]any{"name": "app", "value": fmt.Sprintf("%02d", i), "entry_id": fmt.Sprintf("entry%02d", i)}})
	}
	rows = append(rows, datalog.Tuple{Args: map[string]any{"name": "db", "value": "outside"}})
	witnesses, truncated := checkWitnesses(rows, "empty", scope, model)
	require.True(t, truncated)
	require.Len(t, witnesses, checkWitnessLimit)
	require.Equal(t, "entry00", witnesses[0].EvidenceIDs[0])
	require.False(t, witnesses[0].Advisory)
	reversed := append([]datalog.Tuple{}, rows...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	again, _ := checkWitnesses(reversed, "empty", scope, model)
	require.Equal(t, witnesses, again)
	witnesses, truncated = checkWitnesses([]datalog.Tuple{rows[len(rows)-1], rows[0]}, "empty", scope, model)
	require.False(t, truncated)
	require.False(t, witnesses[0].Advisory)
	require.True(t, witnesses[1].Advisory)
}
func TestCheckWitnessesRedactWithoutChangingOriginalEvidence(t *testing.T) {
	model := &systemmodel.SystemModel{Populate: systemmodel.Populate{SealedInputs: map[string]systemmodel.SealedInput{"TOKEN": {Ref: "pass:secret/token"}}}}
	nested := map[string]any{"provider": "pass:secret/token", "number": json.Number("9007199254740993")}
	rows := []datalog.Tuple{{Args: map[string]any{"nested": nested, "huge": strings.Repeat("x", 5000)}}}
	witnesses, _ := checkWitnesses(rows, "empty", nil, model)
	encoded, err := json.Marshal(witnesses)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "pass:secret/token")
	require.Contains(t, string(encoded), "9007199254740993")
	require.Contains(t, string(encoded), "omitted: value exceeds witness limit")
	require.Equal(t, "pass:secret/token", nested["provider"])
}
func TestNonemptyFailureHasExplanationWithoutInventedWitness(t *testing.T) {
	witnesses, truncated := checkWitnesses(nil, "nonempty", nil, nil)
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
