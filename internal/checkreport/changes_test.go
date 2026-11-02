package checkreport

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/checks"
	"github.com/chazu/pudl/internal/datalog"
)

// evaluation is a recorded `expect: empty` check over a complete snapshot of
// scope gcp/prod whose failing matches are the named firewalls.
func evaluation(t *testing.T, complete bool, names ...string) CheckResult {
	t.Helper()
	var tuples []datalog.Tuple
	for i, name := range names {
		// entry_id varies with every observation; it must not change identity.
		tuples = append(tuples, datalog.Tuple{Args: map[string]interface{}{"project": "prod", "name": name, "entry_id": time.Now().String() + string(rune('a'+i))}})
	}
	c := CheckResult{Name: "closed", Query: "open_ingress", Expect: "empty", Severity: "fail", Scope: checks.ScopeSnapshots,
		Evidence: []acute.ObservationEvidence{{SnapshotID: "snap", Scope: "gcp/prod", Complete: complete}},
		Count:    len(tuples), Passed: len(tuples) == 0, Outcome: "pass"}
	if !c.Passed {
		c.Outcome = "fail"
	}
	c.Witnesses, c.WitnessesTruncated = Witnesses(tuples, c.Expect)
	c.Findings, c.FindingsTruncated = MatchFindings(tuples, c.Expect, c.Outcome, c.Witnesses)
	return c
}

func byState(findings []Finding) map[string][]string {
	out := map[string][]string{}
	for _, f := range findings {
		out[f.State] = append(out[f.State], f.Arguments["name"].(string))
	}
	for _, names := range out {
		sort.Strings(names)
	}
	return out
}

func firstSeen(t *testing.T, findings []Finding, name string) time.Time {
	t.Helper()
	for _, f := range findings {
		if f.Arguments["name"] == name {
			require.NotNil(t, f.FirstSeen)
			return *f.FirstSeen
		}
	}
	t.Fatalf("no finding for %s", name)
	return time.Time{}
}

// TestClassifyBaselineRegressionFix walks a fixture sequence: a baseline with
// one open firewall, a regression that opens another, then a fix of the
// first. Each step is classified against the previous recorded step.
func TestClassifyBaselineRegressionFix(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	t1, t2 := t0.Add(time.Hour), t0.Add(2*time.Hour)

	baseline := NewObservation("run_0", "observe-only", t0, evaluation(t, true, "a"), false)
	findings, changes := Classify(baseline, nil)
	baseline.Check.Findings = findings
	assert.Equal(t, map[string][]string{"new": {"a"}}, byState(findings))
	assert.Equal(t, "no comparable earlier report", changes.Note)
	assert.Equal(t, t0, firstSeen(t, findings, "a"))

	regression := NewObservation("run_1", "observe-only", t1, evaluation(t, true, "a", "b"), false)
	findings, changes = Classify(regression, []Observation{baseline})
	regression.Check.Findings = findings
	assert.Equal(t, map[string][]string{"persisting": {"a"}, "new": {"b"}}, byState(findings))
	assert.Equal(t, Changes{Since: "run_0", SinceAt: &t0, New: 1, Persisting: 1}, *changes)
	assert.Equal(t, t0, firstSeen(t, findings, "a"), "a persisting finding keeps its first-seen time")
	assert.Equal(t, t1, firstSeen(t, findings, "b"))

	fix := NewObservation("run_2", "observe-only", t2, evaluation(t, true, "b"), false)
	findings, changes = Classify(fix, []Observation{regression})
	assert.Equal(t, map[string][]string{"persisting": {"b"}, "resolved": {"a"}}, byState(findings))
	assert.Equal(t, 0, changes.New)
	assert.Equal(t, 1, changes.Resolved)
	assert.Equal(t, t1, firstSeen(t, findings, "b"))
	assert.Equal(t, t0, firstSeen(t, findings, "a"), "a resolved finding says when it was first seen")
}

// TestClassifyIncompleteObservationDoesNotResolve: a partial observation and
// a check without a verdict leave earlier findings unknown, never resolved,
// and a later complete observation still compares with the complete one.
func TestClassifyIncompleteObservationDoesNotResolve(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	baseline := NewObservation("run_0", "observe-only", t0, evaluation(t, true, "a", "b"), false)
	baseline.Check.Findings, _ = Classify(baseline, nil)

	partial := NewObservation("run_1", "observe-only", t0.Add(time.Hour), evaluation(t, false, "b", "c"), false)
	require.False(t, partial.Complete)
	findings, changes := Classify(partial, []Observation{baseline})
	partial.Check.Findings = findings
	assert.Equal(t, map[string][]string{"persisting": {"b"}, "new": {"c"}, "unknown": {"a"}}, byState(findings))
	assert.Equal(t, 0, changes.Resolved)
	assert.Contains(t, changes.Note, "incomplete")

	failedInput := evaluation(t, true)
	failedInput.Outcome, failedInput.Passed = "unknown", false
	findings, changes = Classify(NewObservation("run_2", "observe-only", t0.Add(2*time.Hour), failedInput, false), []Observation{partial, baseline})
	assert.Equal(t, map[string][]string{"unknown": {"a", "b", "c"}}, byState(findings))
	assert.Contains(t, changes.Note, "no verdict")

	// A complete observation after the partial one: c was seen by the partial
	// one, so it is not new; a is resolved only now.
	complete := NewObservation("run_3", "observe-only", t0.Add(3*time.Hour), evaluation(t, true, "c"), false)
	findings, changes = Classify(complete, []Observation{partial, baseline})
	assert.Equal(t, map[string][]string{"persisting": {"c"}, "resolved": {"a", "b"}}, byState(findings))
	assert.Equal(t, t0.Add(time.Hour), firstSeen(t, findings, "c"))
	assert.Equal(t, "run_1", changes.Since)
}

// TestClassifyTruncatedFindingsCannotResolve: with more matches than recorded
// findings, an earlier finding missing from the record may still match.
func TestClassifyTruncatedFindingsCannotResolve(t *testing.T) {
	baseline := NewObservation("run_0", "observe-only", time.Now(), evaluation(t, true, "a"), false)
	current := evaluation(t, true, "b")
	current.FindingsTruncated = true
	findings, changes := Classify(NewObservation("run_1", "observe-only", time.Now(), current, false), []Observation{baseline})
	assert.Equal(t, map[string][]string{"new": {"b"}, "unknown": {"a"}}, byState(findings))
	assert.Equal(t, 0, changes.Resolved)
}

// TestObservationCompleteness: a run-scoped check is as complete as its own
// observation; a latest-known one resolves only when facts change.
func TestObservationCompleteness(t *testing.T) {
	run := CheckResult{Name: "x", Expect: "empty", Scope: checks.ScopeRun, Outcome: "pass", Passed: true}
	assert.False(t, NewObservation("r", "m", time.Now(), run, false).Complete)
	assert.True(t, NewObservation("r", "m", time.Now(), run, true).Complete)
	latest := run
	latest.Scope = checks.ScopeLatestKnown
	assert.True(t, NewObservation("r", "m", time.Now(), latest, false).Complete)
	noEvidence := run
	noEvidence.Scope = checks.ScopeSnapshots
	assert.False(t, NewObservation("r", "m", time.Now(), noEvidence, true).Complete)
}

func TestComparableRequiresTheSameCheckAndScope(t *testing.T) {
	base := NewObservation("r", "observe-only", time.Now(), evaluation(t, true, "a"), false)
	other := base
	assert.True(t, Comparable(base, other))
	other.Check.Query = "other_relation"
	assert.False(t, Comparable(base, other))
	other = base
	other.Check.Evidence = []acute.ObservationEvidence{{Scope: "gcp/staging", Complete: true}}
	assert.False(t, Comparable(base, other))
	other = base
	other.Mode = "observe-only (inventory)"
	assert.False(t, Comparable(base, other), "different modes are not compared")
	assert.Equal(t, "observe-only (inventory) replay", observationMode(&ModelReport{Mode: "observe-only (inventory)", Drift: &acute.ModelDriftResult{}}), "a catalog replay is not compared with a live observation")
}

// TestPolicyViolationsClassifyByResourceAndPolicy: policy_noncompliant
// witnesses are identified by resource and policy, so one resource failing a
// second policy is a new finding.
func TestPolicyViolationsClassifyByResourceAndPolicy(t *testing.T) {
	policyCheck := func(rows ...[2]string) CheckResult {
		var tuples []datalog.Tuple
		for _, row := range rows {
			tuples = append(tuples, datalog.Tuple{Args: map[string]interface{}{"resource": row[0], "policy": row[1], "severity": "high"}})
		}
		c := CheckResult{Name: "policy", Query: "policy_noncompliant", Expect: "empty", Severity: "fail", Scope: checks.ScopeLatestKnown, Outcome: "fail", Count: len(tuples)}
		c.Witnesses, _ = Witnesses(tuples, c.Expect)
		c.Findings, _ = MatchFindings(tuples, c.Expect, c.Outcome, c.Witnesses)
		return c
	}
	before := NewObservation("run_0", "checks-only", time.Now(), policyCheck([2]string{"bucket/a", "public-access"}), false)
	after := NewObservation("run_1", "checks-only", time.Now(), policyCheck([2]string{"bucket/a", "public-access"}, [2]string{"bucket/a", "versioning"}), false)
	findings, changes := Classify(after, []Observation{before})
	assert.Equal(t, 1, changes.New)
	assert.Equal(t, 1, changes.Persisting)
	for _, f := range findings {
		assert.Equal(t, f.Arguments["policy"] == "versioning", f.State == FindingNew)
	}
}

// TestClassifyOlderReportsByWitnesses: a report recorded before findings
// existed is compared by its witnesses, first seen when it was recorded.
func TestClassifyOlderReportsByWitnesses(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	old := evaluation(t, true, "a")
	old.Findings = nil
	for i := range old.Witnesses {
		old.Witnesses[i].Key = ""
	}
	findings, changes := Classify(NewObservation("run_1", "observe-only", time.Now(), evaluation(t, true, "a"), false), []Observation{NewObservation("run_0", "observe-only", t0, old, false)})
	assert.Equal(t, 1, changes.Persisting)
	assert.Equal(t, t0, *findings[0].FirstSeen)
}

func TestFindingKeyIgnoresEvidenceIdentifiers(t *testing.T) {
	a := FindingKey(map[string]any{"name": "fw", "entry_id": "e1", "snapshot_id": "s1"})
	b := FindingKey(map[string]any{"name": "fw", "entry_id": "e2", "snapshot_id": "s2"})
	assert.Equal(t, a, b)
	assert.NotEqual(t, a, FindingKey(map[string]any{"name": "other", "entry_id": "e1"}))
	assert.NotEqual(t, FindingKey(map[string]any{"entry_id": "e1"}), FindingKey(map[string]any{"entry_id": "e2"}),
		"a head projecting only evidence identifiers is identified by them")
}

func TestMatchFindingsBoundsAndNonempty(t *testing.T) {
	var tuples []datalog.Tuple
	for i := 0; i < FindingLimit+5; i++ {
		tuples = append(tuples, datalog.Tuple{Args: map[string]interface{}{"n": i}})
	}
	findings, truncated := MatchFindings(tuples, "empty", "fail", nil)
	assert.Len(t, findings, FindingLimit)
	assert.True(t, truncated)

	findings, truncated = MatchFindings(nil, "nonempty", "fail", nil)
	assert.Len(t, findings, 1)
	assert.False(t, truncated)
	findings, _ = MatchFindings(nil, "nonempty", "unknown", nil)
	assert.Nil(t, findings, "a check without a verdict has no findings")
}
