package checkreport

import (
	"fmt"
	"testing"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/waiver"
	"github.com/stretchr/testify/require"
)

func waiverTuples(n int) []datalog.Tuple {
	var rows []datalog.Tuple
	for i := 0; i < n; i++ {
		rows = append(rows, datalog.Tuple{Args: map[string]any{"name": fmt.Sprintf("r%02d", i)}})
	}
	return rows
}

// waiveBelow waives findings named before limit.
func waiveBelow(limit string) MatchFunc {
	return func(args map[string]any) (*waiver.Ref, error) {
		if args["name"].(string) < limit {
			return &waiver.Ref{Index: 0, Owner: "o", State: waiver.Active}, nil
		}
		return nil, nil
	}
}

// Every finding is matched, not only the kept witnesses; unwaived findings
// are kept first; a check is waived only when every finding is.
func TestSetFindingsWaivesOnlyWhenEveryFindingIs(t *testing.T) {
	c := CheckResult{Name: "c", Expect: "empty", Outcome: "fail", Count: 25}
	require.NoError(t, c.SetFindings(waiverTuples(25), waiveBelow("r20")))
	require.Equal(t, "fail", c.Outcome)
	require.Equal(t, 20, c.Waived)
	require.True(t, c.WitnessesTruncated)
	require.Len(t, c.Witnesses, WitnessLimit)
	require.Equal(t, "r20", c.Witnesses[0].Arguments["name"], "unwaived findings come first")
	require.False(t, c.Witnesses[4].Waived())
	require.True(t, c.Witnesses[5].Waived())
	require.True(t, c.Failing())

	c = CheckResult{Name: "c", Expect: "empty", Outcome: "fail", Count: 25}
	require.NoError(t, c.SetFindings(waiverTuples(25), waiveBelow("r99")))
	require.Equal(t, OutcomeWaived, c.Outcome)
	require.Equal(t, 25, c.Waived)
	require.False(t, c.Failing())
	require.False(t, c.Passed)
}

func TestSummaryCountsWaivedChecksApart(t *testing.T) {
	r := &ModelReport{CompletionStatus: "succeeded", Checks: []CheckResult{
		{Name: "a", Outcome: "pass", Passed: true},
		{Name: "b", Outcome: OutcomeWaived},
	}}
	s := Summarize(r, nil)
	require.Equal(t, "waived", s.Checks)
	require.Equal(t, 1, s.WaivedChecks)
	require.Zero(t, s.FailedChecks)

	r.Checks = append(r.Checks, CheckResult{Name: "c", Outcome: "fail"})
	s = Summarize(r, nil)
	require.Equal(t, "fail", s.Checks)
	require.Equal(t, 1, s.FailedChecks)
}
