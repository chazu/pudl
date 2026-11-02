package checkreport

import (
	"encoding/json"
	"testing"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/waiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ownedReport() *ModelReport {
	failed := CheckResult{Name: "open", Query: "open", Expect: "empty", Severity: "fail", Outcome: "fail", Count: 4, Witnesses: []Witness{
		{Arguments: map[string]any{"resource": "r-orders"}, Resource: "r-orders"},
		{Arguments: map[string]any{"entry_id": "e-payments"}, EvidenceIDs: []string{"e-payments"}},
		{Arguments: map[string]any{"resource_id": "r-unowned"}},
		{Arguments: map[string]any{"router": "selfLink"}},
	}}
	other := CheckResult{Name: "other", Query: "other", Expect: "empty", Severity: "warn", Outcome: "fail", Count: 1, Witnesses: []Witness{
		{Arguments: map[string]any{"resource": "r-orders"}, Resource: "r-orders"},
	}}
	passed := CheckResult{Name: "clean", Query: "clean", Expect: "empty", Severity: "fail", Outcome: "pass", Passed: true}
	report := &ModelReport{ReportVersion: ModelReportVersion, RunID: "run", Model: "m", Mode: "observe-only", Checks: []CheckResult{failed, other, passed}}
	AttributeOwners(report.Checks, func(resources, entries []string) string {
		for _, id := range resources {
			if id == "r-orders" {
				return "orders"
			}
		}
		for _, id := range entries {
			if id == "e-payments" {
				return "payments"
			}
		}
		return ""
	})
	return report
}

func TestAttributeOwnersUsesResourcesThenEntries(t *testing.T) {
	report := ownedReport()
	var owners []string
	for _, w := range report.Checks[0].Witnesses {
		owners = append(owners, w.Owner)
	}
	assert.Equal(t, []string{"orders", "payments", "", ""}, owners)

	encoded, err := json.Marshal(report.Checks[0].Witnesses[3])
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"owner":""`, "every finding carries an owner field")
}

func TestMarkdownGroupsFindingsByOwner(t *testing.T) {
	text := ownedReport().Markdown()
	assert.Contains(t, text, "    - owner orders:\n      - {\"resource\":\"r-orders\"}\n"+
		"    - owner payments:\n      - {\"entry_id\":\"e-payments\"}\n"+
		"    - owner (none):\n      - {\"resource_id\":\"r-unowned\"}\n      - {\"router\":\"selfLink\"}\n")

	waived := ownedReport()
	waived.Checks[0].Witnesses[0].Waiver = &waiver.Ref{Index: 0, Owner: "sre", Reason: "migrating", Expires: "2026-12-01", State: waiver.Active}
	assert.Contains(t, waived.Markdown(), "    - owner orders:\n      - {\"resource\":\"r-orders\"} — waived by waivers[0] (sre, until 2026-12-01): migrating\n",
		"a grouped finding keeps its waiver note; the waiver's owner is not the resource's")

	unattributed := &ModelReport{Checks: []CheckResult{{Name: "x", Expect: "empty", Outcome: "fail", Count: 1, Witnesses: []Witness{{Arguments: map[string]any{"a": "b"}}}}}}
	assert.Contains(t, unattributed.Markdown(), "    - {\"a\":\"b\"}\n", "without owners findings stay a flat list")
	assert.NotContains(t, unattributed.Markdown(), "owner (none)")
}

func TestForOwnerKeepsOnlyThatOwnersFindings(t *testing.T) {
	report := ownedReport()
	payments := report.ForOwner("payments")
	require.Len(t, payments.Checks, 2, "the failed check without a payments finding is left out")
	assert.Equal(t, "open", payments.Checks[0].Name)
	require.Len(t, payments.Checks[0].Witnesses, 1)
	assert.Equal(t, "payments", payments.Checks[0].Witnesses[0].Owner)
	assert.Equal(t, 4, payments.Checks[0].Count, "counts still include every match")
	assert.Equal(t, "clean", payments.Checks[1].Name, "passed checks stay")
	assert.Len(t, report.Checks[0].Witnesses, 4, "the original report is unchanged")

	orders := report.ForOwner("orders")
	require.Len(t, orders.Checks, 3)
	assert.Len(t, orders.Checks[1].Witnesses, 1)
}

func TestCheckForOwnerFiltersEmbeddedMembers(t *testing.T) {
	payload, err := json.Marshal(ownedReport())
	require.NoError(t, err)
	check := &acute.RunSetReport{RunSetID: "check", Members: []acute.RunSetMemberReport{{Model: "m", Report: payload}, {Model: "blocked"}}}
	filtered, err := CheckForOwner(check, "payments")
	require.NoError(t, err)
	member, err := DecodeModelReport(filtered.Members[0].Report)
	require.NoError(t, err)
	require.Len(t, member.Checks, 2)
	assert.Len(t, member.Checks[0].Witnesses, 1)
	assert.Empty(t, filtered.Members[1].Report)
	assert.JSONEq(t, string(payload), string(check.Members[0].Report), "the recorded report is unchanged")
}
