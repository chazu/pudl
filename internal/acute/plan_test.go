package acute

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
)

func TestNewRunPlanResolvesEffectiveScopeBeforeSideEffects(t *testing.T) {
	model := &systemmodel.SystemModel{
		Name:     "example",
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
		Desired: []map[string]any{
			{"name": "web"},
			{"name": "api", "depends_on": []any{"web"}},
		},
	}

	plan, err := NewRunPlan(model, RunRequest{
		Converge: true,
		Only:     []string{"api"},
		MaxIters: 2,
	})

	require.NoError(t, err)
	assert.Same(t, model, plan.Original)
	assert.Len(t, plan.Effective.Desired, 2)
	assert.Len(t, model.Desired, 2)
}

func TestNewRunPlanRejectsUnknownSelector(t *testing.T) {
	model := &systemmodel.SystemModel{
		Name:     "example",
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
		Desired:  []map[string]any{{"name": "web"}},
	}

	_, err := NewRunPlan(model, RunRequest{
		Converge: true,
		Only:     []string{"missing"},
		MaxIters: 1,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not match")
}

func TestNewRunPlanAllowsObserveOnlyWithDefaultIterationCap(t *testing.T) {
	model := &systemmodel.SystemModel{Name: "example"}

	plan, err := NewRunPlan(model, RunRequest{MaxIters: 5})

	require.NoError(t, err)
	assert.Same(t, model, plan.Effective)
	session := NewRunSession(plan)
	assert.NotEmpty(t, session.RunID)
	assert.Same(t, plan, session.Plan)
}
