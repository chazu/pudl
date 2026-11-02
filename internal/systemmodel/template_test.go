package systemmodel

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileTemplate(t *testing.T, source, name string) cue.Value {
	t.Helper()
	ctx := cuecontext.New()
	value := ctx.CompileString(schemaCUE+"\n"+source, cue.Filename("template.cue"))
	require.NoError(t, value.Err())
	instance := value.LookupPath(cue.ParsePath(name))
	require.True(t, instance.Exists())
	return instance
}

const appTemplateSource = `
app: #SystemModel & {
	name: "app"
	depends_on: ["network"]
	populate: runs: [{argv: ["true"]}]
	desired: [{"_schema": "example.application", name: "api"}]
}`

func TestModelTemplateDecodesConcreteModel(t *testing.T) {
	value := compileTemplate(t, appTemplateSource, "app")
	template, err := NewTemplate(value, TemplateOrigin{SchemaName: "models.#App", LoadDir: "/models"})
	require.NoError(t, err)
	assert.Equal(t, "app", template.Name)
	assert.Equal(t, "/models", template.Origin.LoadDir)
	assert.Equal(t, []string{"network"}, template.DependsOn)
	assert.Same(t, value.Context(), template.Context())

	model, err := template.Model()
	require.NoError(t, err)
	require.Len(t, model.Desired, 1)
	assert.Equal(t, "api", model.Desired[0]["name"])
}

func TestModelTemplateCannotDeclareWorkspaceSecretWritePolicy(t *testing.T) {
	source := `
app: #SystemModel & {
	name: "app"
	secrets: writable_refs: ["pass:untrusted/*"]
	populate: runs: [{argv: ["true"]}]
}`
	value := cuecontext.New().CompileString(schemaCUE+"\n"+source, cue.Filename("template.cue"))
	err := value.Err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secrets")
}
