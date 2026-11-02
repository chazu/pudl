package graphexport

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/datalog"
)

func edgeRow(typ string, src, tgt any, basis string) datalog.Tuple {
	return datalog.Tuple{Relation: EdgeRelation, Args: map[string]any{"type": typ, "source": src, "target": tgt, "basis": basis}}
}

func TestResolveReferences(t *testing.T) {
	x := newRefIndex()
	x.add("rid-subnet", []string{"subnet-1"})
	x.add("rid-net-a", []string{"projects/p/global/networks/n"})
	x.add("rid-net-b", []string{"projects/p/global/networks/n"})

	id, problem := x.resolve("rid-subnet")
	require.Equal(t, "rid-subnet", id, "a resource ID resolves to itself")
	require.Empty(t, problem)
	id, _ = x.resolve("subnet-1")
	require.Equal(t, "rid-subnet", id)
	id, problem = x.resolve("sg-missing")
	require.Equal(t, "ref:sg-missing", id, "an uncollected reference stays visible")
	require.Empty(t, problem)
	id, problem = x.resolve("projects/p/global/networks/n")
	require.Equal(t, "ref:projects/p/global/networks/n", id, "an ambiguous reference is not guessed")
	require.Contains(t, problem, "matches 2 resources")
}

func TestBuildRelationships(t *testing.T) {
	x := newRefIndex()
	x.add("rid-i", []string{"i-1"})
	x.add("rid-sg", []string{"sg-1"})
	var warnings []string
	warn := func(s string) { warnings = append(warnings, s) }
	rules := []RuleTuples{
		{Rule: datalog.Rule{Name: "a"}, Tuples: []datalog.Tuple{
			edgeRow("protected_by", "rid-i", "sg-1", "observed"),
			edgeRow("protected_by", "rid-i", "sg-gone", "observed"),
			edgeRow("encrypted_by", "rid-i", nil, "observed"), // absent optional field
		}},
		{Rule: datalog.Rule{Name: "b"}, Tuples: []datalog.Tuple{
			edgeRow("protected_by", "rid-i", "sg-1", "observed"), // same edge, second rule
			edgeRow("reads_from", "rid-i", "sg-1", "maybe"),
		}},
		{Rule: datalog.Rule{Name: "typo"}, Tuples: []datalog.Tuple{edgeRow("in_vpc", "rid-i", nil, "observed")}},
	}
	rels := buildRelationships(rules, x, warn)
	byID := map[string]Relationship{}
	for _, r := range rels {
		byID[r.ID] = r
	}
	require.Len(t, rels, 3)
	merged := byID["protected_by:rid-i->rid-sg"]
	require.Len(t, merged.Evidence, 2, "duplicate edges merge evidence from each rule")
	require.Equal(t, "rule a", merged.Evidence[0].Source)
	require.Contains(t, byID, "protected_by:rid-i->ref:sg-gone")
	require.Equal(t, "unknown", byID["reads_from:rid-i->rid-sg"].Basis, "an invalid basis is downgraded, never upgraded")
	require.Len(t, warnings, 2)
	require.Contains(t, warnings[0], `basis "maybe"`)
	require.Contains(t, warnings[1], "rule typo: all 1 rows have a null source or target")
}
