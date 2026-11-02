package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

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
		byID[r.ID] = r.NativeID
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
	r := runCLI(t, "example", "install", "aws", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	checkPack(t, "aws-fixture-", awsPackTypes, awsPackFamilies, 2)

	g := exportGraph(t)
	if os.Getenv("PUDL_GRAPH_DUMP") != "" {
		require.NoError(t, os.WriteFile(os.Getenv("PUDL_GRAPH_DUMP"), mustJSON(t, g), 0o644))
	}
	l := newGraphLookup(g)
	require.Equal(t, "pudl", g.Meta.Origin)
	require.Empty(t, g.Meta.Warnings)

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
}

func TestGraphExportGCPPack(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "example", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	checkGCPPack(t, 2)

	g := exportGraph(t)
	if os.Getenv("PUDL_GRAPH_DUMP_GCP") != "" {
		require.NoError(t, os.WriteFile(os.Getenv("PUDL_GRAPH_DUMP_GCP"), mustJSON(t, g), 0o644))
	}
	l := newGraphLookup(g)
	require.Empty(t, g.Meta.Warnings)
	for _, r := range g.Resources {
		require.False(t, graphexport.InternalTypes[r.Type], "pudl's own %s records are not infrastructure", r.Type)
	}
	require.Contains(t, l.edges("in_subnet"), "https://container.googleapis.com/v1/projects/example-project/zones/us-east1-b/clusters/legacy-public -> ref:projects/example-project/regions/us-east1/subnetworks/default",
		"a subnet the fixtures never collected stays an explicit reference")
	// Subnets reference networks by trimmed selfLink; refs resolve them.
	subnetEdges := l.edges("in_network")
	require.NotEmpty(t, l.edges("subscribed_to"))
	require.NotEmpty(t, l.edges("nat_ip"))
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
