package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphexport"
)

// exportGraph runs `pudl graph export` and decodes the document.
func exportGraph(t *testing.T) graphexport.Graph {
	t.Helper()
	r := runCLI(t, "graph", "export")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var g graphexport.Graph
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &g), r.Stdout)
	return g
}

// requireCompleteVocabulary checks that a pack draws in pudl ui without any
// TypeScript: the vocabulary has no problems, every exported resource type
// has display info, and every relationship type is declared.
func requireCompleteVocabulary(t *testing.T, g graphexport.Graph) {
	t.Helper()
	v := g.Vocabulary
	require.NotNil(t, v)
	require.Empty(t, v.Problems)
	types := map[string]bool{}
	for _, r := range g.Resources {
		types[r.Type] = true
	}
	var list []string
	for typ := range types {
		list = append(list, typ)
	}
	require.Empty(t, v.MissingDisplay(list), "resource types with no display info")
	declared := map[string]bool{}
	for _, e := range v.EdgeTypes {
		declared[e.Type] = true
	}
	for _, rel := range g.Relationships {
		require.True(t, declared[rel.Type], "relationship type %s is not declared", rel.Type)
	}
	require.NotEmpty(t, v.EdgeTypesIn("containment"))
}

type graphLookup struct {
	g        graphexport.Graph
	byNative map[string]graphexport.Resource
}

func newGraphLookup(g graphexport.Graph) graphLookup {
	l := graphLookup{g: g, byNative: map[string]graphexport.Resource{}}
	for _, r := range g.Resources {
		l.byNative[r.NativeID] = r
	}
	return l
}

// edges returns "type nativeSrc -> nativeOrRefTgt" strings for the relationships of a type.
func (l graphLookup) edges(typ string) []string {
	byID := map[string]string{}
	for _, r := range l.g.Resources {
		if !strings.HasPrefix(r.ID, "ext:") {
			byID[r.ID] = r.NativeID
		}
	}
	name := func(id string) string {
		if n, ok := byID[id]; ok {
			return n
		}
		return id
	}
	var out []string
	for _, rel := range l.g.Relationships {
		if rel.Type == typ {
			out = append(out, name(rel.Source)+" -> "+name(rel.Target))
		}
	}
	return out
}

func TestGraphExportAWSPack(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "aws", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	checkPack(t, "aws-fixture-", awsPackTypes, awsPackFamilies, 2)

	g := exportGraph(t)
	if os.Getenv("PUDL_GRAPH_DUMP") != "" {
		require.NoError(t, os.WriteFile(os.Getenv("PUDL_GRAPH_DUMP"), mustJSON(t, g), 0o644))
	}
	l := newGraphLookup(g)
	require.Equal(t, "pudl", g.Meta.Origin)
	require.Empty(t, g.Meta.Warnings)
	requireCompleteVocabulary(t, g)

	inst := l.byNative["i-0aa00000000000002"]
	require.Equal(t, "aws.ec2.instance", inst.Type)
	require.Equal(t, "aws", inst.Provider)
	require.NotEmpty(t, inst.Account)
	require.NotEmpty(t, inst.Region)
	require.Equal(t, "observed", inst.Evidence[0].Basis, "fixture collectors are observations")
	require.NotEmpty(t, inst.Evidence[0].SnapshotID)

	sg := l.byNative["sg-0aa00000000000002"]
	require.NotEqual(t, sg.NativeID, sg.Name, "security groups are named by GroupName")

	require.Contains(t, l.edges("protected_by"), "i-0aa00000000000002 -> sg-0aa00000000000002")
	require.NotEmpty(t, l.edges("in_subnet"))
	require.NotEmpty(t, l.edges("in_vpc"))
	for _, rel := range g.Relationships {
		require.NotEmpty(t, rel.Evidence)
		require.True(t, strings.HasPrefix(rel.Evidence[0].Source, "rule aws_graph_"), rel.Evidence[0].Source)
	}

	// Security-group ingress: from another group, from a range, and from anywhere.
	ingress := l.edges("allows_ingress_from")
	require.Contains(t, ingress, "sg-0aa00000000000003 -> sg-0aa00000000000004")
	require.Contains(t, ingress, "sg-0aa00000000000003 -> ext:cidr:10.0.0.0/16")
	require.Contains(t, ingress, "sg-0aa00000000000002 -> ext:cidr:0.0.0.0/0")
	require.Contains(t, ingress, "sg-0aa00000000000001 -> ext:cidr:::/0", "IPv6 ranges too")
	bastion := relationship(t, g, "allows_ingress_from", "sg-0aa00000000000002", "ext:cidr:0.0.0.0/0")
	require.Equal(t, map[string]any{"port": float64(22), "protocol": "tcp"}, bastion.Properties)
	web := relationship(t, g, "allows_ingress_from", "sg-0aa00000000000001", "ext:cidr:0.0.0.0/0")
	require.Equal(t, []any{float64(80), float64(443)}, web.Properties["port"], "two rules from one range are one edge")
	anywhere := l.byNative["0.0.0.0/0"]
	require.Equal(t, "external.cidr", anywhere.Type, "an address range is external, not a missing resource")

	// Role trust, in every policy shape the fixtures use.
	trust := l.edges("trust_allows_assume")
	require.Contains(t, trust, "ext:any:* -> arn:aws:iam::123456789012:role/open-trust-test", "a lone statement with Principal \"*\"")
	require.Contains(t, trust, "ext:any:* -> arn:aws:iam::123456789012:role/legacy-cross-account", "\"*\" inside a principal list")
	require.Contains(t, trust, "ref:arn:aws:iam::111122223333:root -> arn:aws:iam::123456789012:role/partner-audit",
		"another account's root is an uncollected principal")
	partner := relationship(t, g, "trust_allows_assume", "ref:arn:aws:iam::111122223333:root", "arn:aws:iam::123456789012:role/partner-audit")
	require.Equal(t, "yes", partner.Properties["conditional"])
	require.NotContains(t, strings.Join(trust, "\n"), "app-ec2-role", "service principals are not projected")

	// Instance -> instance profile -> role.
	profile := "arn:aws:iam::123456789012:instance-profile/app-ec2-role"
	require.Contains(t, l.edges("has_instance_profile"), "i-0aa00000000000001 -> "+profile)
	require.Contains(t, l.edges("profile_role"), profile+" -> arn:aws:iam::123456789012:role/app-ec2-role")

	// Egress: the default allow-all, with protocol -1 and no port.
	egress := relationship(t, g, "allows_egress_to", "sg-0aa00000000000001", "ext:cidr:0.0.0.0/0")
	require.Equal(t, "-1", egress.Properties["protocol"])
	require.NotContains(t, egress.Properties, "port")
}

// relationship finds one edge by type and native (or ref/ext) endpoint names.
func relationship(t *testing.T, g graphexport.Graph, typ, src, tgt string) graphexport.Relationship {
	t.Helper()
	native := map[string]string{}
	for _, r := range g.Resources {
		native[r.ID] = r.NativeID
	}
	name := func(id string) string {
		if n, ok := native[id]; ok && !strings.HasPrefix(id, "ext:") {
			return n
		}
		return id
	}
	for _, rel := range g.Relationships {
		if rel.Type == typ && name(rel.Source) == src && name(rel.Target) == tgt {
			return rel
		}
	}
	t.Fatalf("no %s edge %s -> %s", typ, src, tgt)
	return graphexport.Relationship{}
}

func TestGraphExportGCPPack(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	checkGCPPack(t, 2)

	g := exportGraph(t)
	if os.Getenv("PUDL_GRAPH_DUMP_GCP") != "" {
		require.NoError(t, os.WriteFile(os.Getenv("PUDL_GRAPH_DUMP_GCP"), mustJSON(t, g), 0o644))
	}
	l := newGraphLookup(g)
	require.Empty(t, g.Meta.Warnings)
	requireCompleteVocabulary(t, g)
	cov := g.Meta.Coverage
	require.NotNil(t, cov)
	require.Positive(t, cov.EdgeRules)
	require.Greater(t, cov.Entries, cov.Resources-1)
	for _, h := range cov.Hidden {
		require.Equal(t, "internal", h.Reason, "every pack fixture record has an identity; only pudl's own records are hidden: %+v", h)
	}
	for _, r := range g.Resources {
		require.False(t, graphexport.InternalTypes[r.Type], "pudl's own %s records are not infrastructure", r.Type)
	}
	require.Contains(t, l.edges("in_subnet"), "https://container.googleapis.com/v1/projects/example-project/zones/us-east1-b/clusters/legacy-public -> ref:projects/example-project/regions/us-east1/subnetworks/default",
		"a subnet the fixtures never collected stays an explicit reference")
	// Subnets reference networks by trimmed selfLink; refs resolve them.
	subnetEdges := l.edges("in_network")
	require.NotEmpty(t, l.edges("subscribed_to"))

	// Compute instances: placed by subnet, the service account they run as,
	// and the firewall rules GCP applies to them.
	vm := func(name string) string {
		return "https://www.googleapis.com/compute/v1/projects/example-project/zones/" + name
	}
	fwl := func(name string) string {
		return "https://www.googleapis.com/compute/v1/projects/example-project/global/firewalls/" + name
	}
	bastion := l.byNative[vm("us-central1-a/instances/bastion-1")]
	require.Equal(t, "gcp.compute.instance", bastion.Type)
	require.Equal(t, "us-central1-a", bastion.Zone)
	require.Equal(t, "us-central1", bastion.Region, "derived from the zone")
	require.Contains(t, l.edges("in_subnet"), vm("us-central1-a/instances/bastion-1")+" -> https://www.googleapis.com/compute/v1/projects/example-project/regions/us-central1/subnetworks/app")
	runsAs := l.edges("runs_as")
	require.Contains(t, runsAs, vm("us-central1-a/instances/bastion-1")+" -> ops@example-project.iam.gserviceaccount.com", "a collected service account resolves")
	require.Contains(t, runsAs, vm("us-central1-c/instances/batch-worker-1")+" -> ref:123456789012-compute@developer.gserviceaccount.com", "the default compute account was not collected")

	// IAM bindings: member -> the policy of the resource that grants it, with
	// roles; the project's own policy and its resources' policies alike.
	bindings := l.edges("iam_binding")
	const project = "//cloudresourcemanager.googleapis.com/projects/example-project"
	const bucket = "//storage.googleapis.com/example-project-public-assets"
	const topic = "//pubsub.googleapis.com/projects/example-project/topics/orders"
	policy := l.byNative[project]
	require.Equal(t, "gcp.iam.policy", policy.Type)
	require.Equal(t, "IAM policy · example-project", policy.Name)
	require.Equal(t, "gcp.iam.policy", l.byNative[bucket].Type)
	require.Contains(t, bindings, "ops@example-project.iam.gserviceaccount.com -> "+project, "serviceAccount: members resolve to the account")
	require.Contains(t, bindings, "ref:user:alice@example.com -> "+project)
	require.Contains(t, bindings, "ext:any:* -> "+project, "allUsers is Anyone, not a missing resource")
	require.Contains(t, bindings, "ext:any:* -> "+bucket, "a bucket's own policy")
	require.Contains(t, bindings, "orders-api@example-project.iam.gserviceaccount.com -> "+topic)
	ops := relationship(t, g, "iam_binding", "ops@example-project.iam.gserviceaccount.com", project)
	require.Equal(t, "roles/editor", ops.Properties["role"])
	sre := relationship(t, g, "iam_binding", "ref:group:sre@example.com", project)
	require.Equal(t, "yes", sre.Properties["conditional"])
	applies := l.edges("applies_to")
	require.Contains(t, applies, fwl("allow-ssh-world")+" -> "+vm("us-central1-a/instances/bastion-1"), "matched by target tag")
	require.NotContains(t, applies, fwl("allow-ssh-world")+" -> "+vm("us-central1-b/instances/orders-api-1"), "untagged instance")
	require.Contains(t, applies, fwl("allow-internal")+" -> "+vm("us-central1-b/instances/orders-api-1"), "untargeted rules apply to the whole network")
	require.NotContains(t, strings.Join(applies, "\n"), "allow-rdp-world-disabled", "a disabled rule applies to nothing")
	tagged := relationship(t, g, "applies_to", fwl("allow-ssh-world"), vm("us-central1-a/instances/bastion-1"))
	require.Equal(t, "bastion", tagged.Properties["matched_tag"])
	fw := l.edges("allows_ingress_from")
	require.NotEmpty(t, fw, "enabled ingress firewall rules permit their source ranges")
	for _, e := range fw {
		require.NotContains(t, e, "disabled", "a disabled rule permits nothing: %s", e)
	}
	require.NotEmpty(t, l.edges("nat_ip"))

	// A rule targeting a service account applies to the instances that run
	// as it.
	hc := relationship(t, g, "applies_to", fwl("allow-health-checks-orders"), vm("us-central1-b/instances/orders-api-1"))
	require.Equal(t, "orders-api@example-project.iam.gserviceaccount.com", hc.Properties["matched_service_account"])
	require.NotContains(t, applies, fwl("allow-health-checks-orders")+" -> "+vm("us-central1-a/instances/bastion-1"), "runs as another account")

	// Peerings, what addresses are attached to, encryption, dead letters and
	// the account a Cloud Run service runs as.
	net := func(name string) string {
		return "https://www.googleapis.com/compute/v1/projects/example-project/global/networks/" + name
	}
	require.ElementsMatch(t, []string{net("prod-vpc") + " -> " + net("default"), net("default") + " -> " + net("prod-vpc")}, l.edges("peers_with"))
	require.Equal(t, "ACTIVE", relationship(t, g, "peers_with", net("prod-vpc"), net("default")).Properties["state"])
	require.ElementsMatch(t, []string{
		"https://www.googleapis.com/compute/v1/projects/example-project/regions/us-central1/addresses/nat-ip-1 -> https://www.googleapis.com/compute/v1/projects/example-project/regions/us-central1/routers/prod-nat-router",
		"https://www.googleapis.com/compute/v1/projects/example-project/global/addresses/web-lb-ip -> ref:projects/example-project/global/forwardingRules/web-lb-https",
	}, l.edges("attached_to"), "a forwarding rule the pack does not collect stays a reference")
	require.Contains(t, l.edges("encrypted_by"), "projects/example-project/topics/orders -> projects/example-project/locations/global/keyRings/app/cryptoKeys/data-key")
	require.Equal(t, []string{"projects/example-project/subscriptions/orders-worker -> projects/example-project/topics/orders-dlq"}, l.edges("publishes_to"))
	require.Contains(t, runsAs, "us-central1/orders-api/123456789012 -> orders-api@example-project.iam.gserviceaccount.com")

	// Who may reach public endpoints: a public Cloud SQL instance's
	// authorized networks, and a GKE control plane's enforced ones.
	sql := func(name string) string {
		return "https://sqladmin.googleapis.com/sql/v1beta4/projects/example-project/instances/" + name
	}
	ingress := l.edges("allows_ingress_from")
	require.Contains(t, ingress, sql("legacy-mysql")+" -> ext:cidr:0.0.0.0/0")
	require.Contains(t, ingress, sql("analytics-mysql")+" -> ext:cidr:198.51.100.0/24")
	require.NotContains(t, strings.Join(ingress, "\n"), sql("orders-db"), "no public IP, no authorized-network edge")
	require.Contains(t, ingress, "https://container.googleapis.com/v1/projects/example-project/locations/us-central1/clusters/prod-private -> ext:cidr:192.0.2.0/24")
	require.NotEmpty(t, subnetEdges)
	for _, e := range subnetEdges {
		require.NotContains(t, e, graphexport.RefPrefix, "every network in the fixtures is cataloged: %s", e)
	}
	for _, r := range g.Resources {
		if r.Type == "gcp.compute.subnetwork" {
			require.NotEmpty(t, r.Region)
			require.NotContains(t, r.Region, "/", "region is the last path segment")
			require.NotEmpty(t, r.Account)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	return b
}

func TestGraphExportAsOf(t *testing.T) {
	cliWorkspace(t)
	for _, pack := range []string{"aws", "gcp"} {
		r := runCLI(t, "pack", "install", pack, "--json")
		require.NoError(t, r.Err, r.Stdout+r.Stderr)
	}
	r := runCLI(t, "check", "aws-fixture-vpcs", "aws-fixture-subnets", "aws-fixture-instances", "--json")
	require.Contains(t, []int{0, 2}, exitCodeFor(r.Err), r.Stdout+r.Stderr)
	// Facts are timestamped in whole seconds: keep the GCP writes clear of T.
	time.Sleep(1100 * time.Millisecond)
	before := time.Now().UTC().Format(time.RFC3339)
	time.Sleep(1100 * time.Millisecond)
	r = runCLI(t, "check", "gcp-fixture-networks", "gcp-fixture-subnets", "--json")
	require.Contains(t, []int{0, 2}, exitCodeFor(r.Err), r.Stdout+r.Stderr)

	providers := func(g graphexport.Graph) map[string]int {
		out := map[string]int{}
		for _, res := range g.Resources {
			out[res.Provider]++
		}
		return out
	}
	now := exportGraph(t)
	require.Positive(t, providers(now)["gcp"])
	require.Empty(t, now.Meta.AsOf)

	r = runCLI(t, "graph", "export", "--as-of", before)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var past graphexport.Graph
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &past))
	require.Equal(t, before, past.Meta.AsOf)
	require.Zero(t, providers(past)["gcp"], "GCP was observed after --as-of")
	require.Equal(t, providers(now)["aws"], providers(past)["aws"])
	l := newGraphLookup(past)
	require.NotEmpty(t, l.edges("in_subnet"), "AWS facts were already believed at --as-of")
	require.Empty(t, l.edges("in_network"), "GCP facts were not")

	r = runCLI(t, "graph", "export", "--as-of", "2000-01-01T00:00:00Z")
	require.NoError(t, r.Err)
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &past))
	require.Empty(t, past.Resources, "nothing was recorded in 2000")
}

func TestGraphExportCoverageNamesUnidentifiedSchemas(t *testing.T) {
	cliWorkspace(t)
	require.NoError(t, os.WriteFile("widgets.json", []byte(`[{"widget": "w1", "size": 3}, {"widget": "w2", "size": 4}]`), 0o644))
	r := runCLI(t, "import", "--path", "widgets.json", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	g := exportGraph(t)
	require.Empty(t, g.Resources, "records with no identity are not resources")
	cov := g.Meta.Coverage
	require.NotNil(t, cov)
	require.Equal(t, 2, cov.Entries, "the array wrapper is not counted")
	require.Zero(t, cov.EdgeRules)
	require.Len(t, cov.Hidden, 1)
	require.Equal(t, "no-identity", cov.Hidden[0].Reason)
	require.Equal(t, 2, cov.Hidden[0].Entries)
}
