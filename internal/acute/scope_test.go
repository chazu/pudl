package acute

import (
	"testing"

	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scopeModel(desired ...map[string]any) *systemmodel.SystemModel {
	return &systemmodel.SystemModel{
		Name:    "m",
		Desired: desired,
		Converge: &systemmodel.PluginPlan{
			Plugin: "k8s",
		},
	}
}

func TestTupleScope_UnrestrictedClassifiesNothing(t *testing.T) {
	model := scopeModel(
		map[string]any{"name": "app", "kind": "Deployment"},
		map[string]any{"name": "db", "kind": "StatefulSet"},
	)
	scope := NewTupleScope(model, model)

	assert.False(t, scope.Restricted())
	assert.False(t, scope.Advisory([]string{"app"}))
	assert.False(t, scope.Advisory([]string{"db"}))
	assert.False(t, scope.Advisory([]string{"anything-at-all"}))
}

func TestTupleScope_OutOfScopeRowIsAdvisory(t *testing.T) {
	model := scopeModel(
		map[string]any{"name": "app", "kind": "Deployment"},
		map[string]any{"name": "db", "kind": "StatefulSet"},
	)
	effective, err := ScopeModelForRun(model, []string{"app"})
	require.NoError(t, err)

	scope := NewTupleScope(model, effective)
	require.True(t, scope.Restricted())

	assert.True(t, scope.Advisory([]string{"db"}), "a row naming only the excluded resource is advisory")
	assert.False(t, scope.Advisory([]string{"app"}), "a row naming the selected resource gates")
}

func TestTupleScope_UnresolvableRowGates(t *testing.T) {
	// Fail-safe: a row that names nothing the model declares is of unknown scope,
	// so it must gate rather than be silently excused.
	model := scopeModel(
		map[string]any{"name": "app", "kind": "Deployment"},
		map[string]any{"name": "db", "kind": "StatefulSet"},
	)
	effective, err := ScopeModelForRun(model, []string{"app"})
	require.NoError(t, err)
	scope := NewTupleScope(model, effective)

	assert.False(t, scope.Advisory([]string{"some-pod-nobody-declared"}))
	assert.False(t, scope.Advisory(nil))
	assert.False(t, scope.Advisory([]string{"", "<nil>"}))
}

func TestTupleScope_MixedRowGates(t *testing.T) {
	// A row naming both an in-scope and an out-of-scope resource gates: excusing
	// it would drop a failure that is genuinely about the selected resource.
	model := scopeModel(
		map[string]any{"name": "app", "kind": "Deployment"},
		map[string]any{"name": "db", "kind": "StatefulSet"},
	)
	effective, err := ScopeModelForRun(model, []string{"app"})
	require.NoError(t, err)
	scope := NewTupleScope(model, effective)

	assert.False(t, scope.Advisory([]string{"db", "app"}))
}

func TestTupleScope_TypeValueGatesWhenAnyOfThatTypeSelected(t *testing.T) {
	// `kind` is a type selector, so a row carrying only `Deployment` names every
	// Deployment — including the selected one — and gates.
	model := scopeModel(
		map[string]any{"name": "app", "kind": "Deployment"},
		map[string]any{"name": "web", "kind": "Deployment"},
		map[string]any{"name": "db", "kind": "StatefulSet"},
	)
	effective, err := ScopeModelForRun(model, []string{"app"})
	require.NoError(t, err)
	scope := NewTupleScope(model, effective)

	assert.False(t, scope.Advisory([]string{"Deployment"}))
	assert.True(t, scope.Advisory([]string{"StatefulSet"}), "no StatefulSet is in scope")
}

func TestTupleScope_ShortSchemaNameResolves(t *testing.T) {
	model := scopeModel(
		map[string]any{"name": "app", "_schema": "k8s.#Deployment"},
		map[string]any{"name": "db", "_schema": "k8s.#StatefulSet"},
	)
	effective, err := ScopeModelForRun(model, []string{"app"})
	require.NoError(t, err)
	scope := NewTupleScope(model, effective)

	// A row carrying the canonical schema value resolves against the selected
	// resource, and one carrying only the excluded schema is advisory.
	assert.False(t, scope.Advisory([]string{"k8s.#Deployment"}))
	assert.True(t, scope.Advisory([]string{"k8s.#StatefulSet"}))
}

func TestArgValuesRendersEveryArgument(t *testing.T) {
	values := ArgValues(map[string]interface{}{"resource": "app", "count": 3})
	assert.ElementsMatch(t, []string{"app", "3"}, values)
}
