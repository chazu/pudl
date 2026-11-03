package graphvocab

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/graphspec"
	"github.com/chazu/pudl/internal/validator"
)

const vocabRules = `
cat_containment: {head: {rel: "graph_edge_category", args: {category: "containment", label: "Containment", meaning: "Placement.", order: 6}}}
cat_network: {head: {rel: "graph_edge_category", args: {category: "network", label: "Network config", meaning: "Configured.", order: 1}}}
et_in_network: {head: {rel: "graph_edge_type", args: {type: "in_network", category: "containment", label: "in network", meaning: "Inside."}}}
et_odd: {head: {rel: "graph_edge_type", args: {type: "odd", category: "mystery", label: "odd", meaning: "?"}}}
et_in_network_again: {head: {rel: "graph_edge_type", args: {type: "in_network", category: "network", label: "x", meaning: "y"}}}
prov_gcp: {head: {rel: "graph_provider", args: {provider: "gcp", label: "Google Cloud"}}}
acct_gcp: {head: {rel: "graph_account_type", args: {provider: "gcp", type: "gcp.project"}}}
acct_bad: {head: {rel: "graph_account_type", args: {provider: "external", type: "gcp.folder"}}}
rt_project: {head: {rel: "graph_resource_type", args: {type: "gcp.project", code: "PROJ", label: "Project", class: "admin"}}}
prov_external: {head: {rel: "graph_provider", args: {provider: "external", label: "External"}}}
rt_cidr: {head: {rel: "graph_resource_type", args: {type: "external.cidr", code: "CIDR", label: "Address range", class: "external"}}}
rt_subnet_fact: {head: {rel: "graph_resource_type", args: {type: "gcp.compute.subnetwork", code: "SN", label: "Subnet", class: "network"}}}
edge_rule: {
	head: {rel: "graph_edge", args: {type: "in_subnet", source: "$R", target: "$S", basis: "observed"}}
	body: [{rel: "gcp_x", args: {resource_id: "$R", subnet: "$S"}}]
}
derived_vocab: {
	head: {rel: "graph_provider", args: {provider: "$P", label: "$P"}}
	body: [{rel: "seen", args: {p: "$P"}}]
}
`

func graphMeta(t *testing.T, resourceType, graph string) validator.SchemaMetadata {
	t.Helper()
	spec, err := graphspec.Parse(json.RawMessage(graph))
	require.NoError(t, err)
	return validator.SchemaMetadata{ResourceType: resourceType, Graph: spec}
}

func TestBuildMergesSchemasAndFacts(t *testing.T) {
	rules, err := datalog.ParseRulesFromSource(vocabRules)
	require.NoError(t, err)
	meta := map[string]validator.SchemaMetadata{
		"gcp.#Subnetwork": graphMeta(t, "gcp.compute.subnetwork", `{"display": {"code": "SUBN", "label": "Subnetwork", "class": "network"}, "network": {"group": true, "via": ["in_network"], "region": true}}`),
		"gcp.#SubnetCopy": graphMeta(t, "gcp.compute.subnetwork", `{"display": {"code": "SUB2", "label": "Subnetwork", "class": "network"}}`),
		"gcp.#Child":      {ResourceType: "gcp.compute.child", BaseSchema: "gcp.#Parent"},
		"gcp.#Parent":     graphMeta(t, "gcp.compute.parent", `{"display": {"code": "PAR", "label": "Parent", "class": "compute"}, "port_of": "attached_to"}`),
		"gcp.#Bare":       graphMeta(t, "gcp.compute.bare", `{"name": "name"}`),
		"pudl.#Internal":  graphMeta(t, "system_model", `{"display": {"code": "SM", "label": "Model", "class": "logical"}}`),
	}
	v := Build(Input{
		Schemas:  []string{"gcp.#SubnetCopy", "gcp.#Subnetwork", "gcp.#Child", "gcp.#Parent", "gcp.#Bare", "pudl.#Internal"},
		Metadata: func(s string) (validator.SchemaMetadata, bool) { m, ok := meta[s]; return m, ok },
		Rules:    rules,
		Skip:     map[string]bool{"system_model": true},
	})

	require.Equal(t, []string{"network", "containment"}, []string{v.Categories[0].ID, v.Categories[1].ID}, "categories sorted by order")
	require.Len(t, v.EdgeTypes, 2)
	require.Equal(t, "containment", v.EdgeTypes[0].Category, "first declaration of in_network wins")

	types := map[string]ResourceType{}
	for _, rt := range v.ResourceTypes {
		types[rt.Type] = rt
	}
	require.NotContains(t, types, "system_model")
	require.NotContains(t, types, "gcp.compute.bare", "a graph block with no display or roles adds nothing")
	require.Equal(t, "gcp.#SubnetCopy", types["gcp.compute.subnetwork"].Schema, "lexically first schema wins")
	require.Equal(t, "PAR", types["gcp.compute.child"].Code, "graph block inherited along base_schema")
	require.Equal(t, "CIDR", types["external.cidr"].Code)
	require.Contains(t, types["external.cidr"].Source, "rt_cidr")
	require.Equal(t, &Admin{Owner: true}, types["gcp.project"].Admin, "graph_account_type marks the owner type")

	problems := strings.Join(v.Problems, "\n")
	for _, want := range []string{
		`in_network" is already declared differently`,
		`category "mystery" is not declared`,
		`gcp.#SubnetCopy and gcp.#Subnetwork declare different graph info`,
		`rt_subnet_fact`,
		`role names edge type "attached_to"`,
		`derives graph_edge type "in_subnet"`,
		`graph_provider takes facts only`,
		`type "gcp.folder" does not belong to provider "external"`,
	} {
		require.Contains(t, problems, want)
	}
}

func TestBuildReportsBadFacts(t *testing.T) {
	rules, err := datalog.ParseRulesFromSource(`
a: {head: {rel: "graph_edge_category", args: {category: "x", label: "X", meaning: "m", order: "first"}}}
b: {head: {rel: "graph_resource_type", args: {type: "t.x", code: "TOOLONG", label: "X", class: "data"}}}
c: {head: {rel: "graph_provider", args: {provider: "aws"}}}
d: {head: {rel: "graph_provider", args: {provider: "aws", label: "AWS", colour: "orange"}}}
`)
	require.NoError(t, err)
	v := Build(Input{Rules: rules, Metadata: func(string) (validator.SchemaMetadata, bool) { return validator.SchemaMetadata{}, false }})
	problems := strings.Join(v.Problems, "\n")
	require.Contains(t, problems, "not an integer")
	require.Contains(t, problems, "longer than 5")
	require.Contains(t, problems, `needs argument "label"`)
	require.Contains(t, problems, `no argument "colour"`)
	require.Empty(t, v.Categories)
	require.Empty(t, v.ResourceTypes)

	out, err := json.Marshal(v)
	require.NoError(t, err)
	require.Contains(t, string(out), `"edgeTypes":[]`, "empty lists encode as [] for the UI")
}

func TestMissingDisplay(t *testing.T) {
	v := &Vocabulary{ResourceTypes: []ResourceType{{Type: "a.x", Code: "X"}, {Type: "a.y"}}}
	require.Equal(t, []string{"a.y", "a.z"}, v.MissingDisplay([]string{"a.z", "a.x", "a.y", ""}))
}

func TestNoteMissingDisplay(t *testing.T) {
	v := &Vocabulary{ResourceTypes: []ResourceType{{Type: "a.x", Code: "X"}, {Type: "a.roles"}}}
	v.NoteMissingDisplay([]string{"a.x", "a.roles", "a.none"})
	require.Len(t, v.Problems, 1)
	require.Contains(t, v.Problems[0], "a.none is in the catalog")
}
