package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// waiverCheck is the part of a check result the waiver tests read.
type waiverCheck struct {
	Name      string `json:"name"`
	Outcome   string `json:"outcome"`
	Count     int    `json:"count"`
	Waived    int    `json:"waived"`
	Passed    bool   `json:"passed"`
	Witnesses []struct {
		Arguments map[string]any `json:"arguments"`
		Waiver    *struct {
			Index int    `json:"index"`
			Owner string `json:"owner"`
			State string `json:"state"`
		} `json:"waiver"`
	} `json:"witnesses"`
}

type waiverOutcome struct {
	Check    string `json:"check"`
	Policy   string `json:"policy"`
	Owner    string `json:"owner"`
	Reason   string `json:"reason"`
	Expires  string `json:"expires"`
	State    string `json:"state"`
	Matches  int    `json:"matches"`
	Resource string `json:"resource"`
}

type waiverReport struct {
	Summary struct {
		Checks       string `json:"checks"`
		WaivedChecks int    `json:"waived_checks"`
		FailedChecks int    `json:"failed_checks"`
	} `json:"summary"`
	Members []struct {
		Model  string `json:"model"`
		Result string `json:"result"`
		Report struct {
			Checks  []waiverCheck   `json:"checks"`
			Waivers []waiverOutcome `json:"waivers"`
		} `json:"report"`
	} `json:"members"`
}

// waiverModelSource is a checks-only model over the GCP fixture with one
// Datalog check (a NAT address not in use) and one policy check (high or
// critical violations on firewalls), and the given waivers.
func waiverModelSource(waivers string) string {
	return `package models

import sm "pudl.schemas/pudl/systemmodel@v0"

#WaivedHygiene: sm.#SystemModel & {
	name: "waived-hygiene"
	checks: [{
		name: "nat-ip-not-in-use", query: "gcp_nat_ip_not_in_use", expect: "empty", severity: "fail", message: "NAT address not in use"
		evidence: ["model:gcp-fixture-routers", "model:gcp-fixture-addresses"], max_age: "1h"
	}, {
		name: "policy-high", query: "gcp_policy_noncompliant_high", expect: "empty", severity: "fail", message: "high policy violation"
		evidence: ["model:gcp-fixture-firewalls"], max_age: "1h"
	}]
	waivers: [` + waivers + `]
}
`
}

func waiverEntries(address, resource, created, expires string) string {
	return fmt.Sprintf(`{check: "nat-ip-not-in-use", witness: {address: %q}, owner: "network-team", reason: "address held for failover", created: %q, expires: %q},
		{policy: "gcp.#FirewallBestPractice", rule: "gcp-firewall-world-ingress", resource: %q, owner: "security", reason: "public bastion until the VPN lands", created: %q, expires: %q}`,
		address, created, expires, resource, created, expires)
}

// checkWaivedModel runs the waiver model, requires the --detailed-exitcode
// result, and returns the JSON report and the human output.
func checkWaivedModel(t *testing.T, pudlDir, waivers string, exitCode int) (waiverReport, string) {
	t.Helper()
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "waived_hygiene.cue"), waiverModelSource(waivers))
	reloadSchemas()
	r := runCLI(t, "check", "waived-hygiene", "--json", "--detailed-exitcode")
	require.Equal(t, exitCode, exitCodeFor(r.Err), r.Stdout+r.Stderr)
	var report waiverReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout)
	require.Len(t, report.Members, 1)
	human := runCLI(t, "check", "waived-hygiene", "--detailed-exitcode")
	require.Equal(t, exitCode, exitCodeFor(human.Err), human.Stdout+human.Stderr)
	return report, human.Stdout
}

func waiverChecks(report waiverReport) map[string]waiverCheck {
	out := map[string]waiverCheck{}
	for _, c := range report.Members[0].Report.Checks {
		out[c.Name] = c
	}
	return out
}

func TestCheckWaivers(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "check", "gcp-fixture-firewalls", "gcp-fixture-addresses", "gcp-fixture-routers", "gcp-fixture-network-hygiene-checks", "--json", "--detailed-exitcode")
	require.Equal(t, 2, exitCodeFor(r.Err), r.Stdout+r.Stderr)

	// Unwaived: both checks fail. Their witnesses name what to waive.
	stale := waiverEntries("no-such-address", "no-such-resource", "2026-10-01", "2999-12-31")
	report, human := checkWaivedModel(t, pudlDir, stale, 2)
	checks := waiverChecks(report)
	require.Equal(t, "fail", checks["nat-ip-not-in-use"].Outcome)
	require.Equal(t, "fail", checks["policy-high"].Outcome)
	address, _ := checks["nat-ip-not-in-use"].Witnesses[0].Arguments["address"].(string)
	resource, _ := checks["policy-high"].Witnesses[0].Arguments["resource"].(string)
	require.NotEmpty(t, address)
	require.NotEmpty(t, resource)

	t.Run("stale", func(t *testing.T) {
		waivers := report.Members[0].Report.Waivers
		require.Len(t, waivers, 2)
		for _, w := range waivers {
			require.Equal(t, "stale", w.State)
			require.Zero(t, w.Matches)
		}
		require.Nil(t, checks["nat-ip-not-in-use"].Witnesses[0].Waiver)
		require.Equal(t, "fail", report.Summary.Checks)
		require.Contains(t, human, "stale waivers[0] check nat-ip-not-in-use witness address=\"no-such-address\" (network-team, until 2999-12-31): matches nothing")
		require.Contains(t, human, "stale waivers[1] policy gcp.#FirewallBestPractice rule gcp-firewall-world-ingress resource no-such-resource")
	})

	t.Run("active", func(t *testing.T) {
		report, human := checkWaivedModel(t, pudlDir, waiverEntries(address, resource, "2026-10-01", "2999-12-31"), 0)
		require.Equal(t, "succeeded", report.Members[0].Result)
		for name, c := range waiverChecks(report) {
			require.Equal(t, "waived", c.Outcome, name)
			require.False(t, c.Passed, "a waived check did not pass")
			require.Equal(t, 1, c.Count, name)
			require.Equal(t, 1, c.Waived, name)
			require.Equal(t, "active", c.Witnesses[0].Waiver.State, name)
		}
		require.Equal(t, 1, waiverChecks(report)["policy-high"].Witnesses[0].Waiver.Index)
		for _, w := range report.Members[0].Report.Waivers {
			require.Equal(t, "active", w.State)
			require.Equal(t, 1, w.Matches)
		}
		require.Equal(t, "waived", report.Summary.Checks)
		require.Equal(t, 2, report.Summary.WaivedChecks)
		require.Zero(t, report.Summary.FailedChecks)
		require.Contains(t, human, "~ nat-ip-not-in-use [fail] waived — 1 match(es), all waived")
		require.Contains(t, human, "waived by waivers[0] (network-team, until 2999-12-31): address held for failover")
		require.Contains(t, human, "waived by waivers[1] (security, until 2999-12-31): public bastion until the VPN lands")
		require.Contains(t, human, "active waivers[1] policy gcp.#FirewallBestPractice")
		require.Contains(t, human, "checks: waived")
	})

	t.Run("expired", func(t *testing.T) {
		report, human := checkWaivedModel(t, pudlDir, waiverEntries(address, resource, "2020-01-01", "2020-01-31"), 2)
		require.Equal(t, "failed", report.Members[0].Result)
		for name, c := range waiverChecks(report) {
			require.Equal(t, "fail", c.Outcome, name)
			require.Zero(t, c.Waived, name)
			require.Equal(t, "expired", c.Witnesses[0].Waiver.State, name)
		}
		for _, w := range report.Members[0].Report.Waivers {
			require.Equal(t, "expired", w.State)
			require.Equal(t, 1, w.Matches)
		}
		require.Equal(t, "fail", report.Summary.Checks)
		require.Contains(t, human, "⚠ expired waivers[0] check nat-ip-not-in-use")
		require.Contains(t, human, "(network-team, expired 2020-01-31): 1 finding(s) count again")
		require.Contains(t, human, "waivers[1] (security) expired 2020-01-31; this finding counts again")
	})
}
