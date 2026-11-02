package acute

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func member(name string, dependsOn ...string) RunSetModel {
	return RunSetModel{Name: name, DependsOn: dependsOn}
}

func TestRunSetPlanOrdersProducersFirstWithLexicalTieBreak(t *testing.T) {
	plan, err := NewRunSetPlan([]RunSetModel{
		member("app", "dns", "network"),
		member("network"),
		member("dns"),
		member("audit", "outside-advisory"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"audit", "dns", "network", "app"}, plan.Ordered)
	require.Len(t, plan.Edges, 2)
	assert.Equal(t, RunSetEdge{From: "app", To: "dns", Sources: []string{"declared"}}, plan.Edges[0])
	assert.Equal(t, RunSetEdge{From: "app", To: "network", Sources: []string{"declared"}}, plan.Edges[1])
}

func TestRunSetPlanNeverAddsModelsOutsideTheNamedSet(t *testing.T) {
	plan, err := NewRunSetPlan([]RunSetModel{member("app", "network")})
	require.NoError(t, err)
	assert.Equal(t, []string{"app"}, plan.Ordered)
	assert.Empty(t, plan.Edges)
}

func TestRunSetPlanRejectsDuplicatesSelfDependenciesAndCycles(t *testing.T) {
	_, err := NewRunSetPlan([]RunSetModel{member("app"), member("app")})
	require.ErrorContains(t, err, "more than once")

	_, err = NewRunSetPlan([]RunSetModel{member("app", "app")})
	require.ErrorContains(t, err, "self-dependency")

	_, err = NewRunSetPlan([]RunSetModel{member("a", "b"), member("b", "a")})
	require.ErrorContains(t, err, "cycle")
}

func TestRunSetPlanResolvesRegistryAliases(t *testing.T) {
	plan, err := NewRunSetPlan([]RunSetModel{
		member("consumer", "ProducerDef"),
		{Name: "producer", Aliases: []string{"ProducerDef"}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"producer", "consumer"}, plan.Ordered)
	assert.Equal(t, "producer", plan.Edges[0].To)
}
