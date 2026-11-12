package cmd

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/doctor"
	"github.com/chazu/pudl/internal/graphexport"
)

// pulumiLinkedGCP maps a fixture state resource to the GCP pack schema of
// the live record it stands for, through a same_resource key: gcp.resource
// by relative name, k8s.object by UID.
var pulumiLinkedGCP = map[string]string{
	"urn:pulumi:prod::example-network::example:index:Network$gcp:compute/network:Network::prod-vpc":      "gcp.#Network",
	"urn:pulumi:prod::example-network::gcp:compute/address:Address::nat-ip-1":                            "gcp.#Address",
	"urn:pulumi:prod::example-network::gcp:compute/router:Router::prod-nat":                              "gcp.#Router",
	"urn:pulumi:prod::example-network::gcp:compute/globalForwardingRule:GlobalForwardingRule::web-https": "gcp.#ForwardingRule",
	pulumiProdGKE:    "gcp.#GKECluster",
	pulumiProdBucket: "gcp.#Bucket",
	"urn:pulumi:prod::example-network::gcp:serviceaccount/account:Account::ops": "gcp.#ServiceAccount",
	pulumiDevDeployment: "gcp.#K8sDeployment",
}

// pulumiUnlinked are state resources with no live partner: a subnet and a
// network the GCP fixtures do not hold, and a type the pack does not name.
var pulumiUnlinked = []string{
	pulumiProdSubnet,
	"urn:pulumi:prod::example-network::gcp:compute/network:Network::shared",
	"urn:pulumi:prod::example-network::gcp:compute/firewall:Firewall::allow-internal",
}

// TestPulumiPackLinksToGCP installs both packs, collects their fixtures and
// checks that each Pulumi state resource with a known GCP id is one group
// with the GCP pack's record of the live resource, and nothing else is.
func TestPulumiPackLinksToGCP(t *testing.T) {
	pudlDir := cliWorkspace(t)
	require.NoError(t, runCLI(t, "pack", "install", "gcp").Err)
	require.NoError(t, runCLI(t, "pack", "install", "pulumi").Err)
	r := runCLI(t, "check", "gcp-fixture-networks", "gcp-fixture-subnets", "gcp-fixture-addresses",
		"gcp-fixture-routers", "gcp-fixture-forwarding-rules", "gcp-fixture-gke-clusters", "gcp-fixture-buckets",
		"gcp-fixture-service-accounts", "gcp-fixture-k8s-deployments", "pulumi-fixture-resources",
		"pulumi-fixture-stacks", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "list", "--json", "--all", "--schema", "pulumi.#Resource")
	require.NoError(t, r.Err, r.Stderr)
	var listed struct {
		Entries []struct {
			ID         string `json:"id"`
			StoredPath string `json:"stored_path"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &listed), r.Stdout)
	entryByURN := map[string]string{}
	for _, e := range listed.Entries {
		raw, err := os.ReadFile(e.StoredPath)
		require.NoError(t, err)
		var rec struct {
			URN string `json:"urn"`
		}
		require.NoError(t, json.Unmarshal(raw, &rec))
		entryByURN[rec.URN] = e.ID
	}
	require.Len(t, entryByURN, 21)

	for urn, schema := range pulumiLinkedGCP {
		shown := showSameResource(t, entryByURN[urn])
		require.NotNil(t, shown, urn)
		require.Len(t, shown.Others, 1, urn)
		require.Equal(t, schema, shown.Others[0].Schema, urn)
	}
	for _, urn := range pulumiUnlinked {
		require.Nil(t, showSameResource(t, entryByURN[urn]), urn)
	}

	// Graph export: Pulumi records draw only as representations. The
	// unlinked ones and the stacks are hidden, counted as member-only, and
	// a linked node carries the Pulumi inspector details.
	g := exportGraph(t)
	hidden := map[string]int{}
	for _, h := range g.Meta.Coverage.Hidden {
		if h.Reason == "member-only" {
			hidden[h.Schema] = h.Entries
		}
	}
	require.Equal(t, map[string]int{"pulumi.#Resource": 13, "pulumi.#Stack": 2}, hidden)
	var network, deployment *graphexport.Resource
	for i, res := range g.Resources {
		require.NotContains(t, []string{"pulumi.#Resource", "pulumi.#Stack"}, res.Schema, "a Pulumi record is never a node of its own")
		for _, rep := range res.Representations {
			switch rep.EntryID {
			case entryByURN["urn:pulumi:prod::example-network::example:index:Network$gcp:compute/network:Network::prod-vpc"]:
				network = &g.Resources[i]
			case entryByURN[pulumiDevDeployment]:
				deployment = &g.Resources[i]
			}
		}
	}
	require.NotNil(t, network)
	require.Equal(t, "gcp.#Network", network.Schema)
	require.Equal(t, []graphexport.Detail{
		{Section: "Pulumi", Key: "commit", Value: "89abcdef0123456789abcdef0123456789abcdef"},
		{Section: "Pulumi", Key: "commit note", Value: "approximate: the update ran with uncommitted changes"},
		{Section: "Pulumi", Key: "component", Value: "example-network-prod", Rank: 0},
		{Section: "Pulumi", Key: "component", Value: "core", Rank: 1},
		{Section: "Pulumi", Key: "repo", Value: "example-owner/example-infra"},
		{Section: "Pulumi", Key: "root", Value: "stacks/network"},
		{Section: "Pulumi", Key: "stack", Value: "example-org/example-network/prod"},
		{Section: "Pulumi", Key: "type", Value: "gcp:compute/network:Network"},
	}, network.Details)
	require.NotNil(t, deployment)
	var keys []string
	for _, d := range deployment.Details {
		keys = append(keys, d.Key)
	}
	require.Equal(t, []string{"commit", "component", "component", "repo", "root", "stack", "type"}, keys, "a clean update has no commit note")

	// doctor: both packs declare gcp.resource; every group has two members.
	check := doctor.CheckSameResource(workspaceSameResource(context.Background(), pudlDir))
	require.Equal(t, "ok", check.Status, "%s\n%s", check.Message, check.Details)
	require.Contains(t, check.Message, "8 group(s)")
}

func showSameResource(t *testing.T, entryID string) *sameResourceGroup {
	t.Helper()
	require.NotEmpty(t, entryID)
	r := runCLI(t, "show", entryID, "--json", "--meta-only")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var shown showOutput
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &shown), r.Stdout)
	return shown.SameResource
}
