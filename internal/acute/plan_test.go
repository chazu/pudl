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

// convergentModel builds a minimal convergent model around the given desired set.
func convergentModel(desired ...map[string]any) *systemmodel.SystemModel {
	return &systemmodel.SystemModel{
		Name:     "example",
		Converge: &systemmodel.PluginPlan{Plugin: "k8s"},
		Desired:  desired,
	}
}

// A dependency that matches one resource by name and another by kind must not
// silently resolve to whichever was declared first: doing so pulls an unnamed
// resource into converge scope and drops the real dependency.
func TestScopeRejectsCrossClassDependencyMatch(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "decoy", "kind": "nginx"},
		map[string]any{"name": "nginx", "kind": "Deployment"},
		map[string]any{"name": "app", "kind": "Deployment", "depends_on": []any{"nginx"}},
	)

	_, err := ScopeModelForRun(model, []string{"app"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
	assert.Contains(t, err.Error(), "name=nginx")
	assert.Contains(t, err.Error(), "name=decoy via kind")
}

// The same ambiguity reached through a user-supplied selector rather than a
// dependency: --only nginx must not quietly select the resource whose *kind*
// happens to be nginx.
func TestScopeRejectsCrossClassSelectorMatch(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "decoy", "kind": "nginx"},
		map[string]any{"name": "nginx", "kind": "Deployment"},
	)

	_, err := ScopeModelForRun(model, []string{"nginx"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

// Two resources sharing an identity name cannot be told apart, so the selector
// must fail rather than converge both.
func TestScopeRejectsDuplicateIdentitySelector(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "web", "path": "a"},
		map[string]any{"name": "web", "path": "b"},
	)

	_, err := ScopeModelForRun(model, []string{"web"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "by identity")
}

// Selecting a set by type is documented behaviour and must keep working.
func TestScopeAllowsTypeSelectorMatchingManyResources(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "web", "kind": "Deployment"},
		map[string]any{"name": "api", "kind": "Deployment"},
		map[string]any{"name": "cfg", "kind": "ConfigMap"},
	)

	scoped, err := ScopeModelForRun(model, []string{"Deployment"})

	require.NoError(t, err)
	assert.Len(t, scoped.Desired, 2)
}

// A dependency naming a type is rejected as a *class*, not on cardinality: a
// type key that happens to match one resource today would silently become two
// edges when a second resource of that type is added.
func TestScopeRejectsDependencyMatchingASet(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "web", "kind": "Deployment"},
		map[string]any{"name": "api", "kind": "Deployment"},
		map[string]any{"name": "app", "kind": "StatefulSet", "depends_on": []any{"Deployment"}},
	)

	_, err := ScopeModelForRun(model, []string{"app"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "names a type, not a resource")
	assert.Contains(t, err.Error(), "identity key")
}

// An unambiguous dependency still resolves, and the closure is transitive.
func TestScopeResolvesUnambiguousTransitiveDependencies(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "db"},
		map[string]any{"name": "web", "depends_on": []any{"db"}},
		map[string]any{"name": "app", "depends_on": []any{"web"}},
	)

	scoped, err := ScopeModelForRun(model, []string{"app"})

	require.NoError(t, err)
	assert.Len(t, scoped.Desired, 3)
}

// A resource matching a selector by both an identity and a type key is still
// unambiguous, because either reading names the same single resource.
func TestScopeAllowsSelfConsistentDualClassMatch(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "nginx", "kind": "nginx"},
		map[string]any{"name": "other", "kind": "Deployment"},
	)

	scoped, err := ScopeModelForRun(model, []string{"nginx"})

	require.NoError(t, err)
	require.Len(t, scoped.Desired, 1)
	assert.Equal(t, "nginx", scoped.Desired[0]["name"])
}

// Mutual dependencies must terminate rather than spin the closure loop.
func TestScopeTerminatesOnCyclicDependencies(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "a", "depends_on": []any{"b"}},
		map[string]any{"name": "b", "depends_on": []any{"a"}},
	)

	scoped, err := ScopeModelForRun(model, []string{"a"})

	require.NoError(t, err)
	assert.Len(t, scoped.Desired, 2)
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

// A type key matching exactly ONE resource is still rejected. The old rule
// checked cardinality, so this resolved — and would have kept resolving right up
// until someone added a second Deployment, at which point one declared edge
// silently became two. Rejecting the key class is what makes the rule stable as
// the model grows.
func TestScopeRejectsASingleMatchTypeDependency(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "web", "kind": "Deployment"},
		map[string]any{"name": "app", "kind": "StatefulSet", "depends_on": []any{"Deployment"}},
	)

	_, err := ScopeModelForRun(model, []string{"app"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "names a type, not a resource")
}

// The typed field has one spelling. A camelCase `dependsOn`, which the untyped
// reader used to accept, is no longer a dependency — and CUE rejects the shapes
// this no longer reads, so a model using them fails at load rather than having
// its dependency silently dropped at plan time.
func TestScopeReadsOnlyTheDeclaredDependsOnField(t *testing.T) {
	model := convergentModel(
		map[string]any{"name": "db"},
		map[string]any{"name": "app", "dependsOn": []any{"db"}},
	)

	scoped, err := ScopeModelForRun(model, []string{"app"})
	require.NoError(t, err)
	assert.Len(t, scoped.Desired, 1, "only the named resource is in scope")
}
