package graphexport

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphvocab"
)

// vpc <-in_vpc- subnet <-in_subnet- i1 -protected_by-> sg ; i2 -in_subnet-> subnet ; i1 -reads-> db ; db -> ref:key
func testGraph() *Graph {
	res := func(id, name string) Resource {
		return Resource{ID: id, Name: name, Type: "t." + name, NativeID: "n-" + name}
	}
	rel := func(typ, s, t string) Relationship {
		return Relationship{ID: typ + ":" + s + "->" + t, Type: typ, Source: s, Target: t}
	}
	return &Graph{
		Resources: []Resource{res("vpc", "vpc"), res("subnet", "subnet"), res("i1", "web-1"), res("i2", "web-2"), res("sg", "sg"), res("db", "orders-db")},
		Relationships: []Relationship{
			rel("in_vpc", "subnet", "vpc"), rel("in_subnet", "i1", "subnet"), rel("in_subnet", "i2", "subnet"),
			rel("protected_by", "i1", "sg"), rel("reads_from", "i1", "db"), rel("encrypted_by", "db", "ref:key"),
		},
	}
}

func ids(g *Graph) []string {
	var out []string
	for _, r := range g.Resources {
		out = append(out, r.ID)
	}
	sort.Strings(out)
	return out
}

func TestSubgraphNeighbourhoodAndClosure(t *testing.T) {
	g := testGraph()
	// db at depth 1 reaches i1; the containment closure then adds subnet and vpc.
	sub := Subgraph(g, []string{"db"}, 1, []string{"in_subnet", "in_vpc"})
	require.Equal(t, []string{"db", "i1", "subnet", "vpc"}, ids(sub))
	require.True(t, sub.Meta.Partial)
	require.Equal(t, &Totals{Resources: 6, Relationships: 6}, sub.Meta.Totals)
	require.Equal(t, 3, sub.Meta.Degree["i1"], "degree counts relationships to unloaded resources too")

	var rels []string
	for _, r := range sub.Relationships {
		rels = append(rels, r.ID)
	}
	require.ElementsMatch(t, []string{
		"in_vpc:subnet->vpc", "in_subnet:i1->subnet", "reads_from:i1->db",
		"encrypted_by:db->ref:key", // a real unresolved reference stays
	}, rels, "edges to unloaded resources (sg, i2) are left out, never turned into references")
}

func TestSubgraphDepthZeroAndUnknownFocus(t *testing.T) {
	sub := Subgraph(testGraph(), []string{"sg", "nope"}, 0, nil)
	require.Equal(t, []string{"sg"}, ids(sub))
	require.Empty(t, sub.Relationships)
	require.Equal(t, 1, sub.Meta.Degree["sg"])
}

func TestSearchRanksNameFirst(t *testing.T) {
	g := testGraph()
	hits := Search(g, "web", 10)
	require.Len(t, hits, 2)
	require.Equal(t, "web-1", hits[0].Name)
	require.Len(t, Search(g, "n-orders", 10), 1, "native IDs match")
	require.Len(t, Search(g, "t.", 3), 3, "limit applies")
	require.Empty(t, Search(g, "  ", 10))
}

func TestSubgraphKeepsVocabulary(t *testing.T) {
	g := testGraph()
	g.Vocabulary = &graphvocab.Vocabulary{EdgeTypes: []graphvocab.EdgeType{{Type: "in_vpc", Category: "containment"}}}
	sub := Subgraph(g, []string{"i1"}, 1, []string{"in_vpc"})
	require.Same(t, g.Vocabulary, sub.Vocabulary)
}
