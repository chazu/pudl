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

func TestExternalEndpointsAreNotMissingResources(t *testing.T) {
	x := newRefIndex()
	x.add("rid-sg", []string{"sg-1"})
	rels := buildRelationships([]RuleTuples{{Rule: datalog.Rule{Name: "r"}, Tuples: []datalog.Tuple{
		edgeRow("allows_ingress_from", "rid-sg", "0.0.0.0/0", "observed"),
		edgeRow("allows_ingress_from", "rid-sg", "10.0.0.0/16", "observed"),
		edgeRow("trust_allows_assume", "*", "rid-sg", "observed"),
		edgeRow("allows_ingress_from", "rid-sg", "not-a-range", "observed"),
	}}}, x, func(string) {})
	targets := map[string]bool{}
	for _, r := range rels {
		targets[r.Target] = true
		targets[r.Source] = true
	}
	require.True(t, targets["ext:cidr:0.0.0.0/0"])
	require.True(t, targets["ext:cidr:10.0.0.0/16"])
	require.True(t, targets["ext:any:*"])
	require.True(t, targets["ref:not-a-range"], "anything else unresolved stays a reference")
	ext := x.externalResources()
	require.Len(t, ext, 3)
	require.Equal(t, "Anywhere (0.0.0.0/0)", ext[1].Name)
	require.Equal(t, "external.principal", ext[0].Type)
}

func TestEdgePropertiesMergePerKey(t *testing.T) {
	x := newRefIndex()
	x.add("rid-sg", []string{"sg-1"})
	rels := buildRelationships([]RuleTuples{{Rule: datalog.Rule{Name: "e"}, Tuples: []datalog.Tuple{
		edgeRow("allows_ingress_from", "rid-sg", "0.0.0.0/0", "observed"),
	}}}, x, func(string) {})
	prop := func(key string, value any) datalog.Tuple {
		return datalog.Tuple{Args: map[string]any{"type": "allows_ingress_from", "source": "sg-1", "target": "0.0.0.0/0", "key": key, "value": value}}
	}
	var warnings []string
	applyProperties(rels, []RuleTuples{{Tuples: []datalog.Tuple{
		prop("port", float64(443)), prop("port", float64(80)), prop("port", float64(443)),
		prop("protocol", "tcp"), prop("port", nil),
		{Args: map[string]any{"type": "in_vpc", "source": "sg-1", "target": "vpc-x", "key": "k", "value": "v"}},
	}}}, x, func(s string) { warnings = append(warnings, s) })
	require.Equal(t, map[string]any{"port": []any{float64(80), float64(443)}, "protocol": "tcp"}, rels[0].Properties)
	require.Equal(t, []string{"1 graph_edge_property rows name an edge no graph_edge rule derived"}, warnings)
}

func TestRegionOfZone(t *testing.T) {
	for zone, region := range map[string]string{"us-central1-a": "us-central1", "us-east-1a": "us-east-1", "europe-west1-b": "europe-west1", "us-central1": "", "": "", "global": ""} {
		require.Equal(t, region, regionOfZone(zone), zone)
	}
}
