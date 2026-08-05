package acute

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
)

func runSetTemplate(name string, bindings, declared []string) *systemmodel.ModelTemplate {
	return &systemmodel.ModelTemplate{Name: name, BindingProducers: bindings, DependsOn: declared}
}

func TestRunSetPlanOrdersProducersFirstWithLexicalTieBreak(t *testing.T) {
	plan, err := NewRunSetPlan([]RunSetModel{
		{Template: runSetTemplate("app", []string{"network"}, []string{"dns"})},
		{Template: runSetTemplate("network", nil, nil)},
		{Template: runSetTemplate("dns", nil, nil)},
		{Template: runSetTemplate("audit", nil, []string{"outside-advisory"})},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"audit", "dns", "network", "app"}, plan.Ordered)
	require.Len(t, plan.Edges, 2)
	assert.Equal(t, RunSetEdge{From: "app", To: "dns", Sources: []string{"declared"}}, plan.Edges[0])
	assert.Equal(t, RunSetEdge{From: "app", To: "network", Sources: []string{"binding"}}, plan.Edges[1])
}

func TestRunSetPlanRequiresBindingProducerInsideExactSet(t *testing.T) {
	_, err := NewRunSetPlan([]RunSetModel{{Template: runSetTemplate("app", []string{"network"}, nil)}})
	require.ErrorContains(t, err, "outside the explicit run set")
}

func TestRunSetPlanRejectsDuplicatesSelfDependenciesAndCycles(t *testing.T) {
	_, err := NewRunSetPlan([]RunSetModel{
		{Template: runSetTemplate("app", nil, nil)},
		{Template: runSetTemplate("app", nil, nil)},
	})
	require.ErrorContains(t, err, "duplicate")

	_, err = NewRunSetPlan([]RunSetModel{{Template: runSetTemplate("app", nil, []string{"app"})}})
	require.ErrorContains(t, err, "self-dependency")

	_, err = NewRunSetPlan([]RunSetModel{
		{Template: runSetTemplate("a", []string{"b"}, nil)},
		{Template: runSetTemplate("b", []string{"a"}, nil)},
	})
	require.ErrorContains(t, err, "cycle")
}

func TestRunSetPlanResolvesRegistryAliases(t *testing.T) {
	plan, err := NewRunSetPlan([]RunSetModel{
		{Template: runSetTemplate("consumer", []string{"ProducerDef"}, nil)},
		{Template: runSetTemplate("producer", nil, nil), Aliases: []string{"ProducerDef"}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"producer", "consumer"}, plan.Ordered)
	assert.Equal(t, "producer", plan.Edges[0].To)
}
