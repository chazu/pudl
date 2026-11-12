package graphexport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/graphspec"
	"github.com/chazu/pudl/internal/validator"
)

func memberOnlyMeta(schemas ...string) MetadataFunc {
	return func(schema string) (validator.SchemaMetadata, bool) {
		for _, s := range schemas {
			if s == schema {
				return validator.SchemaMetadata{Graph: &graphspec.Spec{MemberOnly: true}}, true
			}
		}
		return validator.SchemaMetadata{Graph: &graphspec.Spec{}}, true
	}
}

// A member_only record draws only as a representation of a group; alone it
// is dropped and counted per schema.
func TestDropMemberOnlyKeepsGroupedRecords(t *testing.T) {
	items := []builtResource{
		{r: Resource{ID: "g1", Schema: "gcp.#Network", Representations: []Representation{{ID: "g1", Schema: "gcp.#Network"}, {ID: "p1", Schema: "pulumi.#Resource"}}}},
		{r: Resource{ID: "p2", Schema: "pulumi.#Resource"}},
		{r: Resource{ID: "p3", Schema: "pulumi.#Resource"}},
		{r: Resource{ID: "s1", Schema: "pulumi.#Stack"}},
		{r: Resource{ID: "n1", Schema: "gcp.#Network"}},
	}
	out, dropped := dropMemberOnly(items, memberOnlyMeta("pulumi.#Resource", "pulumi.#Stack"))
	var ids []string
	for _, it := range out {
		ids = append(ids, it.r.ID)
	}
	assert.Equal(t, []string{"g1", "n1"}, ids)
	assert.Equal(t, map[string]int{"pulumi.#Resource": 2, "pulumi.#Stack": 1}, dropped)
}

// graph_detail rows attach to the resource they name, through a grouped
// member's ID, deduplicated and ordered by section, key, rank and value.
func TestApplyDetailsResolvesMembersAndOrders(t *testing.T) {
	resources := []Resource{{ID: "g1"}, {ID: "n1"}}
	index := newRefIndex()
	index.add("g1", nil)
	index.add("n1", nil)
	index.alias("p1", "g1")
	row := func(args map[string]any) datalog.Tuple { return datalog.Tuple{Args: args} }
	rows := []RuleTuples{{Tuples: []datalog.Tuple{
		row(map[string]any{"resource_id": "p1", "section": "Pulumi", "key": "component", "value": "subnets", "rank": int64(3)}),
		row(map[string]any{"resource_id": "p1", "section": "Pulumi", "key": "component", "value": "network", "rank": int64(2)}),
		row(map[string]any{"resource_id": "p1", "section": "Pulumi", "key": "component", "value": "network", "rank": int64(2)}),
		row(map[string]any{"resource_id": "p1", "section": "Pulumi", "key": "stack", "value": "org/proj/prod"}),
		row(map[string]any{"resource_id": "g1", "section": "Audit", "key": "owner", "value": "team-a"}),
		row(map[string]any{"resource_id": "p1", "section": "Pulumi", "key": "missing", "value": nil}),
		row(map[string]any{"resource_id": "zz", "section": "Pulumi", "key": "stack", "value": "x"}),
	}}}
	var warnings []string
	applyDetails(resources, rows, index, func(s string) { warnings = append(warnings, s) })
	require.Equal(t, []Detail{
		{Section: "Audit", Key: "owner", Value: "team-a"},
		{Section: "Pulumi", Key: "component", Value: "network", Rank: 2},
		{Section: "Pulumi", Key: "component", Value: "subnets", Rank: 3},
		{Section: "Pulumi", Key: "stack", Value: "org/proj/prod"},
	}, resources[0].Details)
	assert.Empty(t, resources[1].Details)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "1 graph_detail rows name a resource that is not in the graph")
}
