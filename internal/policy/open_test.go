package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gcpTypes are draft types shaped like the GCP pack's (pudl-opz).
const gcpTypes = `package gcp

#Firewall: {
	_pudl: {schema_type: "base", resource_type: "gcp.compute.firewall", identity_fields: ["selfLink"]}
	selfLink:      string
	direction?:    string
	sourceRanges?: [...string]
	...
}

#GKECluster: {
	_pudl: {schema_type: "base", resource_type: "gcp.container.cluster", identity_fields: ["selfLink"]}
	selfLink:  string
	endpoint?: string
	privateClusterConfig?: {...}
	...
}
`

// gcpPolicies are the reproductions from the GCP pack: a comprehension whose
// condition reads an optional field, and a reference into a closed nested
// struct.
const gcpPolicies = `package gcp

#FirewallWorldIngress: {
	_pudl: {schema_type: "policy", base_schema: "gcp.#Firewall"}
	direction?:    _
	sourceRanges?: _ @pudl(rule="r", severity="high")
	if direction == "INGRESS" {sourceRanges?: [...!="0.0.0.0/0"]}
}

#GKEPrivateEndpoint: {
	_pudl: {schema_type: "policy", base_schema: "gcp.#GKECluster"}
	privateClusterConfig: {
		enablePrivateNodes:     true @pudl(rule="private-nodes", severity="high")
		enablePrivateEndpoint?: _
	}
	endpoint?: _ @pudl(rule="private-endpoint", severity="high")
	if privateClusterConfig.enablePrivateEndpoint == _|_ {endpoint?: "needs a private endpoint"}
}
`

func loadGCP(t *testing.T, policies string) *Set {
	t.Helper()
	set, errs := loadDir(t, writeModule(t, map[string]string{
		"gcp/types.cue":    gcpTypes,
		"gcp/policies.cue": policies,
	}))
	require.Empty(t, errs)
	return set
}

func TestLoad_UndecidableComprehensionKeepsRules(t *testing.T) {
	set := loadGCP(t, gcpPolicies)
	p, ok := set.Policy("gcp.#FirewallWorldIngress")
	require.True(t, ok)
	require.Equal(t, []Rule{{Name: "r", Severity: SeverityHigh, Paths: []string{"sourceRanges"},
		DeclaredBy: "gcp.#FirewallWorldIngress"}}, p.Rules)

	record := map[string]any{"selfLink": "fw", "direction": "INGRESS", "sourceRanges": []any{"10.0.0.0/8", "0.0.0.0/0"}}
	res, err := set.Evaluate(p, "gcp.#Firewall", record)
	require.NoError(t, err)
	assert.Equal(t, StatusNoncompliant, res.Status)
	assert.Equal(t, []Violation{{Rule: "r", Severity: SeverityHigh, Path: "sourceRanges[*]",
		Kind: KindConflict, DeclaredBy: "gcp.#FirewallWorldIngress"}}, res.Violations)

	record["direction"] = "EGRESS"
	res, err = set.Evaluate(p, "gcp.#Firewall", record)
	require.NoError(t, err)
	assert.Equal(t, StatusCompliant, res.Status)

	delete(record, "direction")
	res, err = set.Evaluate(p, "gcp.#Firewall", record)
	require.NoError(t, err)
	assert.Equal(t, StatusCompliant, res.Status, "a condition the record leaves undecided applies nothing: %v", res.Violations)
}

func TestLoad_RuleOnlyInsideComprehensionFailsToLoad(t *testing.T) {
	_, errs := loadDir(t, writeModule(t, map[string]string{
		"gcp/types.cue": gcpTypes,
		"gcp/policies.cue": `package gcp

#Hidden: {
	_pudl: {schema_type: "policy", base_schema: "gcp.#Firewall"}
	direction?: _
	if direction == "INGRESS" {sourceRanges?: [...!="0.0.0.0/0"] @pudl(rule="r", severity="high")}
}
`,
	}))
	require.Len(t, errs, 1)
	assert.Equal(t, "gcp.#Hidden", errs[0].Policy)
	assert.Contains(t, errs[0].Error(), `sourceRanges: @pudl(rule="r", severity="high") cannot be applied`)
}

func TestEvaluate_NestedClosednessIsIgnored(t *testing.T) {
	set := loadGCP(t, gcpPolicies)
	p, _ := set.Policy("gcp.#GKEPrivateEndpoint")
	cluster := func(pcc map[string]any) map[string]any {
		return map[string]any{"selfLink": "c", "endpoint": "203.0.113.10", "privateClusterConfig": pcc,
			"unmodelled": map[string]any{"deep": 1}}
	}

	res, err := set.Evaluate(p, "gcp.#GKECluster", cluster(map[string]any{
		"enablePrivateNodes": true, "enablePrivateEndpoint": true, "masterIpv4CidrBlock": "172.16.0.0/28",
	}))
	require.NoError(t, err)
	assert.Equal(t, StatusCompliant, res.Status, "extra nested fields must not break references: %v", res.Violations)

	res, err = set.Evaluate(p, "gcp.#GKECluster", cluster(map[string]any{
		"enablePrivateNodes": false, "masterIpv4CidrBlock": "172.16.0.0/28",
	}))
	require.NoError(t, err)
	assert.Equal(t, []Violation{
		{Rule: "private-endpoint", Severity: SeverityHigh, Path: "endpoint", Kind: KindConflict, DeclaredBy: "gcp.#GKEPrivateEndpoint"},
		{Rule: "private-nodes", Severity: SeverityHigh, Path: "privateClusterConfig.enablePrivateNodes", Kind: KindConflict, DeclaredBy: "gcp.#GKEPrivateEndpoint"},
	}, res.Violations, "real constraints still fail beside extra nested fields")
}
