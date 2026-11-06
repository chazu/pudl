package graphexport

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphspec"
	"github.com/chazu/pudl/internal/graphvocab"
)

// org <- folder <- proj-a, proj-b (gcp.project, owners); vm-1, vm-2 in proj-a; vm-3 in proj-b;
// an aws instance naming account "proj-a" is not owned by the GCP project.
func ownersGraph() *Graph {
	res := func(id, typ, provider, native, account string) Resource {
		return Resource{ID: id, Name: id, Type: typ, Provider: provider, NativeID: native, Account: account}
	}
	rt := func(typ string, admin *graphspec.Admin) graphvocab.ResourceType {
		return graphvocab.ResourceType{Type: typ, Roles: graphspec.Roles{Admin: admin}}
	}
	return &Graph{
		Resources: []Resource{
			res("org", "gcp.organization", "gcp", "organizations/1", ""),
			res("folder", "gcp.folder", "gcp", "folders/2", ""),
			res("pa", "gcp.project", "gcp", "proj-a", "proj-a"),
			res("pb", "gcp.project", "gcp", "proj-b", "proj-b"),
			res("vm-1", "gcp.compute.instance", "gcp", "vm-1", "proj-a"),
			res("vm-2", "gcp.compute.instance", "gcp", "vm-2", "proj-a"),
			res("vm-3", "gcp.compute.instance", "gcp", "vm-3", "proj-b"),
			res("ec2", "aws.ec2.instance", "aws", "i-1", "proj-a"),
		},
		Relationships: []Relationship{
			{ID: "f", Type: "in_org", Source: "folder", Target: "org"},
			{ID: "a", Type: "in_folder", Source: "pa", Target: "folder"},
			{ID: "b", Type: "in_folder", Source: "pb", Target: "folder"},
		},
		Vocabulary: &graphvocab.Vocabulary{ResourceTypes: []graphvocab.ResourceType{
			rt("gcp.organization", &graphspec.Admin{}),
			rt("gcp.folder", &graphspec.Admin{Parent: []string{"in_org"}}),
			rt("gcp.project", &graphspec.Admin{Owner: true, Parent: []string{"in_folder"}}),
			rt("gcp.compute.instance", nil),
		}},
	}
}

func sorted(s []string) []string {
	sort.Strings(s)
	return s
}

func TestOverviewIDsAreAdministrative(t *testing.T) {
	require.Equal(t, []string{"folder", "org", "pa", "pb"}, sorted(OverviewIDs(ownersGraph())))
	require.Empty(t, OverviewIDs(&Graph{Resources: []Resource{{ID: "x"}}}), "no vocabulary: nothing is administrative")
}

func TestOwnedIDsFollowTheAccountAttribute(t *testing.T) {
	g := ownersGraph()
	require.Equal(t, []string{"vm-1", "vm-2"}, sorted(OwnedIDs(g, []string{"pa"})), "same provider only; the owner itself is not listed")
	require.Equal(t, []string{"vm-1", "vm-2", "vm-3"}, sorted(OwnedIDs(g, []string{"pa", "pb"})))
	require.Empty(t, OwnedIDs(g, []string{"vm-1", "nope"}), "non-owners own nothing")
}

func TestSubgraphReportsOwnedCounts(t *testing.T) {
	g := ownersGraph()
	sub := Subgraph(g, OverviewIDs(g), 0, nil)
	require.Equal(t, map[string]int{"pa": 2, "pb": 1}, sub.Meta.Owned)
	sub = Subgraph(g, []string{"vm-1"}, 0, nil)
	require.Nil(t, sub.Meta.Owned, "no owner present")
}
