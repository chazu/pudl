package checkreport

import (
	"encoding/json"
	"strings"
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

// Classified findings carry owners too, and their change listing groups and
// filters by owner like witnesses.
func TestFindingsCarryOwners(t *testing.T) {
	c := CheckResult{Name: "open", Expect: "empty", Outcome: "fail", Count: 3,
		Changes: &Changes{New: 2, Resolved: 1},
		Findings: []Finding{
			{Key: "a", State: FindingNew, Arguments: map[string]any{"resource": "r-orders"}},
			{Key: "b", State: FindingNew, Arguments: map[string]any{"entry_id": "e-payments"}},
			{Key: "c", State: FindingResolved},
		}}
	AttributeOwners([]CheckResult{c}, func(resources, entries []string) string {
		if len(resources) > 0 && resources[0] == "r-orders" {
			return "orders"
		}
		if len(entries) > 0 && entries[0] == "e-payments" {
			return "payments"
		}
		return ""
	})
	assert.Equal(t, []string{"orders", "payments", ""}, []string{c.Findings[0].Owner, c.Findings[1].Owner, c.Findings[2].Owner},
		"a finding without recorded arguments has no owner")

	var b strings.Builder
	WriteChanges(&b, c)
	assert.Contains(t, b.String(), "    - by owner: orders 1, payments 1\n"+
		"    - new {\"resource\":\"r-orders\"}\n"+
		"    - new {\"entry_id\":\"e-payments\"}\n", "owners are counted; findings are one flat list")

	report := (&ModelReport{Checks: []CheckResult{c}}).ForOwner("payments")
	require.Len(t, report.Checks, 1)
	require.Len(t, report.Checks[0].Findings, 1)
	assert.Equal(t, "b", report.Checks[0].Findings[0].Key)
	assert.Empty(t, (&ModelReport{Checks: []CheckResult{c}}).ForOwner("search").Checks)
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

// Every finding carries its resource and owner, including those past the
// witness limit and waived ones, which are listed last and so are the first
// to fall past it (pudl-upc). Arguments stay recorded for witnesses only.
func TestEveryFindingCarriesResourceAndOwner(t *testing.T) {
	const n = WitnessLimit + 3
	c := CheckResult{Name: "c", Expect: "empty", Outcome: "fail", Count: n}
	require.NoError(t, c.SetFindings(waiverTuples(n), waiveBelow("r01")))
	require.True(t, c.WitnessesTruncated)
	AttributeOwners([]CheckResult{c}, func(resources, entries []string) string {
		if len(resources) > 0 {
			return "team-" + resources[0]
		}
		return ""
	})

	require.Len(t, c.Findings, n)
	withArgs := 0
	for _, f := range c.Findings {
		require.NotEmpty(t, f.Resource, "finding %s", f.Key)
		assert.Equal(t, "team-"+f.Resource, f.Owner, "finding %s", f.Key)
		if f.Arguments != nil {
			withArgs++
		}
		if f.Resource == "r00" {
			assert.True(t, f.Waived)
			assert.Nil(t, f.Arguments, "the waived finding fell past the witness limit")
		}
	}
	assert.Equal(t, WitnessLimit, withArgs)

	encoded, err := json.Marshal(c.Findings[0])
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "refs")
	owned := (&ModelReport{Checks: []CheckResult{c}}).ForOwner("team-r22")
	require.Len(t, owned.Checks, 1)
	assert.Len(t, owned.Checks[0].Findings, 1, "owner filtering keeps a finding past the witness limit")
}
