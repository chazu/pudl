package waiver

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func checkWaiver(witness map[string]any, expires string) Waiver {
	return Waiver{Check: "open", Witness: witness, Owner: "sre", Reason: "bastion", Created: "2020-01-01", Expires: expires}
}

func policyWaiver(rule, expires string) Waiver {
	return Waiver{Policy: "gcp.#BucketBestPractice", Rule: rule, Resource: "r1", Owner: "data", Reason: "legacy", Created: "2020-01-01", Expires: expires}
}

func TestExpiredAppliesThroughTheExpiresDate(t *testing.T) {
	w := Waiver{Expires: "2026-11-01"}
	require.False(t, w.Expired(now), "applies on its expires date")
	require.False(t, w.Expired(time.Date(2026, 11, 1, 23, 59, 59, 0, time.UTC)))
	require.True(t, w.Expired(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)), "stops the next day")
}

func TestCheckWaiverMatchesEverySelectorValue(t *testing.T) {
	m := NewMatcher([]Waiver{checkWaiver(map[string]any{"host": "a", "port": 22}, "2026-12-31")}, now, nil)
	ref, err := m.Match("open", map[string]any{"host": "a", "port": int64(22), "proto": "tcp"})
	require.NoError(t, err)
	require.Equal(t, &Ref{Index: 0, Owner: "sre", Reason: "bastion", Expires: "2026-12-31", State: Active}, ref)

	for _, args := range []map[string]any{{"host": "a", "port": int64(23)}, {"host": "a"}} {
		ref, err = m.Match("open", args)
		require.NoError(t, err)
		require.Nil(t, ref, "%v", args)
	}
	ref, _ = m.Match("other", map[string]any{"host": "a", "port": 22.0})
	require.Nil(t, ref, "another check's witnesses are not waived")
	require.Equal(t, []Outcome{{Waiver: checkWaiver(map[string]any{"host": "a", "port": 22}, "2026-12-31"), State: Active, Matches: 1}}, m.Outcomes())
}

func TestExpiredAndStaleOutcomes(t *testing.T) {
	waivers := []Waiver{
		checkWaiver(map[string]any{"host": "a"}, "2026-10-31"), // expired, matches
		checkWaiver(map[string]any{"host": "z"}, "2026-12-31"), // stale
		checkWaiver(map[string]any{"host": "z"}, "2026-10-31"), // expired, matches nothing
	}
	m := NewMatcher(waivers, now, nil)
	ref, err := m.Match("open", map[string]any{"host": "a"})
	require.NoError(t, err)
	require.Equal(t, Expired, ref.State, "an expired waiver is reported, not applied")
	var states []string
	for _, o := range m.Outcomes() {
		states = append(states, fmt.Sprintf("%s:%d", o.State, o.Matches))
	}
	require.Equal(t, []string{"expired:1", "stale:0", "expired:0"}, states)
}

func TestActiveWaiverWinsOverExpired(t *testing.T) {
	m := NewMatcher([]Waiver{checkWaiver(map[string]any{"host": "a"}, "2026-10-31"), checkWaiver(map[string]any{"host": "a"}, "2026-12-31")}, now, nil)
	ref, err := m.Match("open", map[string]any{"host": "a"})
	require.NoError(t, err)
	require.Equal(t, Active, ref.State)
	require.Equal(t, 1, ref.Index)
}

// A policy finding is waived only when every violation it stands for has an
// active waiver; severity or rule arguments on the witness narrow which.
func TestPolicyWaiverCoversEveryViolation(t *testing.T) {
	violations := []Violation{
		{Rule: "gcp-bucket-public-access-prevention", Severity: "high"},
		{Rule: "gcp-bucket-uniform-access", Severity: "medium"},
	}
	lookups := 0
	lookup := func(resource, policy string) ([]Violation, error) {
		lookups++
		require.Equal(t, "r1", resource)
		return violations, nil
	}
	finding := func(severity string) map[string]any {
		args := map[string]any{"resource": "r1", "policy": "gcp.#BucketBestPractice"}
		if severity != "" {
			args["severity"] = severity
		}
		return args
	}

	m := NewMatcher([]Waiver{policyWaiver("gcp-bucket-uniform-access", "2026-12-31")}, now, lookup)
	ref, err := m.Match("policy-any", finding("medium"))
	require.NoError(t, err)
	require.Equal(t, Active, ref.State, "the medium finding stands for the waived rule only")
	ref, err = m.Match("policy-any", finding("high"))
	require.NoError(t, err)
	require.Nil(t, ref, "the high finding's violation has no waiver")
	ref, err = m.Match("policy-high", finding(""))
	require.NoError(t, err)
	require.Nil(t, ref, "a finding without severity stands for both violations")
	require.Equal(t, 1, lookups, "violations are looked up once per resource and policy")

	m = NewMatcher([]Waiver{policyWaiver("gcp-bucket-uniform-access", "2026-12-31"), policyWaiver("gcp-bucket-public-access-prevention", "2026-10-31")}, now, lookup)
	ref, err = m.Match("policy-high", finding(""))
	require.NoError(t, err)
	require.Equal(t, Expired, ref.State, "one violation's waiver expired: the finding counts and the expiry is flagged")

	m = NewMatcher([]Waiver{policyWaiver("gcp-bucket-uniform-access", "2026-12-31")}, now, func(string, string) ([]Violation, error) { return nil, nil })
	ref, err = m.Match("policy-any", finding("medium"))
	require.NoError(t, err)
	require.Nil(t, ref, "no current violation: nothing to waive")
	require.Equal(t, Stale, m.Outcomes()[0].State)
}

func TestPolicyLookupErrorPropagates(t *testing.T) {
	m := NewMatcher([]Waiver{policyWaiver("x", "2026-12-31")}, now, func(string, string) ([]Violation, error) { return nil, fmt.Errorf("boom") })
	_, err := m.Match("c", map[string]any{"resource": "r1", "policy": "gcp.#BucketBestPractice"})
	require.ErrorContains(t, err, "boom")
}

func TestTarget(t *testing.T) {
	require.Equal(t, `check open witness host="a" port=22`, checkWaiver(map[string]any{"port": 22, "host": "a"}, "").Target())
	require.Equal(t, "policy gcp.#BucketBestPractice rule x resource r1", policyWaiver("x", "").Target())
}
