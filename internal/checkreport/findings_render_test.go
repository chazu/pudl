package checkreport

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The shape of a live IAM hygiene run: two checks over the same keys.
func iamChecks() []CheckResult {
	key := func(id, project, owner string) Finding {
		name := "projects/" + project + "/serviceAccounts/sa@" + project + ".iam.gserviceaccount.com/keys/" + id
		return Finding{Key: "k-" + id, State: FindingNew, ResourceID: "rid-" + id, Account: project, Owner: owner,
			Type: "gcp.iam.service_account_key", Resource: name, Name: name}
	}
	older := CheckResult{Name: "sa-key-older-than-90d", Expect: "empty", Outcome: "fail", Count: 3,
		Changes:  &Changes{New: 3},
		Findings: []Finding{key("a1", "prod-payments", "payments"), key("b2", "prod-payments", "payments"), key("c3", "test-a", "")}}
	never := CheckResult{Name: "sa-key-never-expires", Expect: "empty", Outcome: "fail", Count: 3,
		Changes:  &Changes{New: 3},
		Findings: []Finding{key("a1", "prod-payments", "payments"), key("b2", "prod-payments", "payments"), key("d4", "prod-api", "")}}
	return []CheckResult{older, never}
}

func TestReportGroupsFindingsByProjectAndOwner(t *testing.T) {
	text := (&ModelReport{Model: "gcp-iam-hygiene-checks", Checks: iamChecks()}).Markdown()

	older := text[strings.Index(text, "sa-key-older-than-90d"):strings.Index(text, "sa-key-never-expires")]
	assert.Contains(t, older, "    - by project (2): prod-payments 2, test-a 1\n")
	assert.Contains(t, older, "    - by owner: payments 2, (none) 1\n")
	assert.Contains(t, older, "    - new …/keys/c3\n", "a key only this check found is listed, shortened")
	assert.Contains(t, older, "    - 2 also fail other checks; see resources failing several checks\n")
	assert.NotContains(t, older, "keys/a1", "keys several checks found are listed once, below")
	assert.NotContains(t, text, `{"`, "no raw JSON")

	assert.Contains(t, text, "### resources failing several checks (2)\n"+
		"  - prod-payments (2)\n"+
		"    - …/keys/a1: sa-key-older-than-90d, sa-key-never-expires\n"+
		"    - …/keys/b2: sa-key-older-than-90d, sa-key-never-expires\n")
}

func TestReportWithoutAccountsOrOwnersHasNoSummary(t *testing.T) {
	c := CheckResult{Name: "open", Expect: "empty", Outcome: "fail", Count: 1,
		Findings: []Finding{{Key: "k", Arguments: map[string]any{"a": "b"}}}}
	text := (&ModelReport{Checks: []CheckResult{c}}).Markdown()
	assert.NotContains(t, text, "by project")
	assert.NotContains(t, text, "by owner")
	assert.NotContains(t, text, "resources failing several checks")
	assert.Contains(t, text, "    - {\"a\":\"b\"}\n", "an unresolved finding is named by its arguments")
}

func TestReportListsAtMostTenFindingsPerCheck(t *testing.T) {
	var findings []Finding
	for _, id := range strings.Split("a b c d e f g h i j k l", " ") {
		findings = append(findings, Finding{Key: id, Name: "res-" + id, Account: "p"})
	}
	c := CheckResult{Name: "many", Expect: "empty", Outcome: "fail", Count: 12, Findings: findings}
	text := (&ModelReport{Checks: []CheckResult{c}}).Markdown()
	assert.Contains(t, text, "    - res-j\n")
	assert.NotContains(t, text, "res-k")
	assert.Contains(t, text, "    - … 2 more; --json lists them\n")
	assert.Contains(t, text, "    - by account (1): p 12\n", "untyped findings are grouped by account")
}

type fakeResolver map[string]string

func (f fakeResolver) Resolve(args map[string]any) (string, string, bool) {
	for _, v := range args {
		if s, ok := v.(string); ok && f[s] != "" {
			return f[s], s, true
		}
	}
	return "", "", false
}

func (f fakeResolver) Describe(id string) Description {
	return Description{Account: "proj-" + id, Type: "gcp.x", Name: "name-" + id}
}

func TestAttributeResourcesUsesEveryArgument(t *testing.T) {
	results := []CheckResult{{Findings: []Finding{
		{Key: "1", refs: map[string]any{"key": "keys/k1", "account": "sa@p"}},
		{Key: "2", Arguments: map[string]any{"key": "keys/unknown"}},
	}}}
	AttributeResources(results, fakeResolver{"keys/k1": "rid1"})
	f := results[0].Findings
	assert.Equal(t, "rid1", f[0].ResourceID)
	assert.Equal(t, "keys/k1", f[0].Resource)
	assert.Equal(t, "proj-rid1", f[0].Account)
	assert.Equal(t, "gcp.x", f[0].Type)
	assert.Equal(t, "name-rid1", f[0].Name)
	assert.Empty(t, f[1].ResourceID)

	AttributeOwners(results, func(ids, _ []string) string {
		if len(ids) > 0 && ids[0] == "rid1" {
			return "payments"
		}
		return ""
	})
	assert.Equal(t, "payments", f[0].Owner, "the owner comes from the resolved resource")
}
