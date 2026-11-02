package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// policyListTypes: a validated bucket type and a draft queue type.
const policyListTypes = `package aws

#Bucket: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.bucket"
		identity_fields: ["Name"]
		review: {status: "validated"}
	}
	Name: string
	...
}

#Queue: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.queue"
		identity_fields: ["QueueName"]
	}
	QueueName: string
	...
}
`

// policyListPolicies: #BucketPCI extends #BucketBP; #QueueBP is on the draft
// type. %s is the PCI encryption rule's severity.
const policyListPolicies = `package aws

#BucketBP: {
	_pudl: {schema_type: "policy", base_schema: "pudl/aws.#Bucket"}
	Public: false @pudl(rule="no-public", severity="high", doc="Buckets are private")
}

#BucketPCI: {
	_pudl: {schema_type: "policy", base_schema: "pudl/aws.#BucketBP"}
	Encrypted: true @pudl(rule="encrypted", severity="%s")
}

#QueueBP: {
	_pudl: {schema_type: "policy", base_schema: "pudl/aws.#Queue"}
	Encrypted: true @pudl(rule="queue-encrypted", severity="low")
}
`

func policyJSON[T any](t *testing.T, args ...string) T {
	t.Helper()
	r := runCLI(t, append(args, "--json")...)
	require.NoError(t, r.Err, r.Stderr)
	var out T
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
	return out
}

func policyByName(t *testing.T, rows []policyRow, name string) policyRow {
	t.Helper()
	for _, r := range rows {
		if r.Policy == name {
			return r
		}
	}
	require.Failf(t, "policy not listed", "%s in %+v", name, rows)
	return policyRow{}
}

func TestPolicyListShowAndCompliance(t *testing.T) {
	pudlDir := cliWorkspace(t)
	awsDir := filepath.Join(pudlDir, "schema", "pudl", "aws")
	writeFile(t, filepath.Join(awsDir, "types.cue"), policyListTypes)
	writeFile(t, filepath.Join(awsDir, "policies.cue"), strings.Replace(policyListPolicies, "%s", "critical", 1))
	const bp, pci, queueBP = "pudl/aws.#BucketBP", "pudl/aws.#BucketPCI", "pudl/aws.#QueueBP"

	// Before any evaluation: every policy, empty counts.
	list := policyJSON[policyListOutput](t, "policy", "list")
	assert.Equal(t, 1, list.ReportVersion)
	require.Len(t, list.Policies, 3)
	p := policyByName(t, list.Policies, pci)
	assert.Equal(t, bp, p.Extends)
	assert.Equal(t, 2, p.Rules, "inherited rules count")
	assert.Equal(t, "validated", p.Review)
	assert.Zero(t, p.LastEvaluated)
	assert.Zero(t, p.Compliant+p.Noncompliant+p.Unknown+p.Stale)
	assert.Equal(t, "draft", policyByName(t, list.Policies, queueBP).Review)
	r := runCLI(t, "policy", "list")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "POLICY")
	assert.Regexp(t, `pudl/aws\.#BucketPCI\s+pudl/aws\.#Bucket\s+pudl/aws\.#BucketBP\s+2\s+validated\s+never\s+0\s+0\s+0\s+0`, r.Stdout)

	writeFile(t, "buckets.json", `[
		{"Name":"ok","Public":false,"Encrypted":true},
		{"Name":"open","Public":true,"Encrypted":false},
		{"Name":"plain","Public":false,"Encrypted":false}
	]`)
	writeFile(t, "queues.json", `[{"QueueName":"q1","Encrypted":false}]`)
	require.NoError(t, runCLI(t, "import", "buckets.json", "--schema", "pudl/aws.#Bucket").Err)
	require.NoError(t, runCLI(t, "import", "queues.json", "--schema", "pudl/aws.#Queue").Err)

	list = policyJSON[policyListOutput](t, "policy", "list")
	p = policyByName(t, list.Policies, pci)
	assert.Equal(t, [3]int{1, 2, 0}, [3]int{p.Compliant, p.Noncompliant, p.Unknown})
	assert.Positive(t, p.LastEvaluated)
	p = policyByName(t, list.Policies, bp)
	assert.Equal(t, [3]int{2, 1, 0}, [3]int{p.Compliant, p.Noncompliant, p.Unknown})
	p = policyByName(t, list.Policies, queueBP)
	assert.Equal(t, 1, p.Noncompliant)
	assert.Equal(t, 1, p.Provisional)

	typed := policyJSON[policyListOutput](t, "policy", "list", "--type", "pudl/aws.#Queue")
	require.Len(t, typed.Policies, 1)
	assert.Equal(t, queueBP, typed.Policies[0].Policy)

	// show: chain, rules with their declaring policy and failing counts, the
	// most severe failing resource first.
	show := policyJSON[policyShowOutput](t, "policy", "show", pci)
	assert.Equal(t, []string{pci, bp, "pudl/aws.#Bucket"}, show.Chain)
	require.Len(t, show.Rules, 2)
	assert.Equal(t, "encrypted", show.Rules[0].Name)
	assert.Equal(t, pci, show.Rules[0].DeclaredBy)
	assert.Equal(t, 2, show.Rules[0].Failing)
	assert.Equal(t, "no-public", show.Rules[1].Name)
	assert.Equal(t, bp, show.Rules[1].DeclaredBy)
	assert.Equal(t, "Buckets are private", show.Rules[1].Doc)
	assert.Equal(t, 1, show.Rules[1].Failing)
	assert.Equal(t, 2, show.FailingTotal)
	require.Len(t, show.Failing, 2)
	assert.Len(t, show.Failing[0].Violations, 2, "open fails both rules")
	assert.Equal(t, "critical", string(show.Failing[0].HighestSeverity))
	openEntry := show.Failing[0].EntryID

	r = runCLI(t, "policy", "show", pci, "--limit", "1")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "Chain: pudl/aws.#BucketPCI -> pudl/aws.#BucketBP -> pudl/aws.#Bucket")
	assert.Contains(t, r.Stdout, "high     no-public (declared by pudl/aws.#BucketBP): 1 failing")
	assert.Contains(t, r.Stdout, "Buckets are private")
	assert.Contains(t, r.Stdout, "Failing resources (1 of 2):")
	assert.Contains(t, r.Stdout, "Results: 1 compliant, 2 noncompliant, 0 unknown, 0 stale")

	r = runCLI(t, "policy", "show", "pudl/aws.#Missing")
	require.Error(t, r.Err)

	// pudl show: each applicable policy with status, severity and violations.
	entry := policyJSON[struct {
		Compliance []complianceResult `json:"compliance"`
	}](t, "show", openEntry)
	require.Len(t, entry.Compliance, 2)
	c := entry.Compliance[1]
	assert.Equal(t, pci, c.Policy)
	assert.Equal(t, "noncompliant", c.Status)
	assert.False(t, c.Provisional)
	assert.Equal(t, "critical", string(c.HighestSeverity))
	require.Len(t, c.Violations, 2)
	assert.Equal(t, "Encrypted", c.Violations[0].Path)
	assert.Equal(t, "Public", c.Violations[1].Path)
	assert.Equal(t, bp, c.Violations[1].DeclaredBy, "an inherited rule names its declaring policy")
	assert.Equal(t, "conflict", string(c.Violations[1].Kind))
	r = runCLI(t, "show", openEntry)
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "Compliance:\n  pudl/aws.#BucketBP: noncompliant, highest severity high\n")
	assert.Contains(t, r.Stdout, "high     no-public at Public (conflict, declared by pudl/aws.#BucketBP)")

	queue := policyJSON[policyShowOutput](t, "policy", "show", queueBP)
	require.Len(t, queue.Failing, 1)
	r = runCLI(t, "show", queue.Failing[0].EntryID)
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "pudl/aws.#QueueBP: noncompliant (provisional: type is a draft), highest severity low")

	// A changed policy makes its stored results stale; a new one is not yet
	// evaluated.
	writeFile(t, filepath.Join(awsDir, "policies.cue"), strings.Replace(policyListPolicies, "%s", "high", 1))
	writeFile(t, filepath.Join(awsDir, "later.cue"), `package aws

#BucketZ: {
	_pudl: {schema_type: "policy", base_schema: "pudl/aws.#Bucket"}
	Name: =~"^b-"
}
`)
	reloadSchemas()
	list = policyJSON[policyListOutput](t, "policy", "list")
	assert.Equal(t, 3, policyByName(t, list.Policies, pci).Stale)
	assert.Zero(t, policyByName(t, list.Policies, bp).Stale)
	entry = policyJSON[struct {
		Compliance []complianceResult `json:"compliance"`
	}](t, "show", openEntry)
	require.Len(t, entry.Compliance, 3)
	assert.True(t, entry.Compliance[1].Stale)
	assert.Equal(t, "not_evaluated", entry.Compliance[2].Status)
}
