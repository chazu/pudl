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

// Only run transforms are exempt from concreteness: an incomplete value
// anywhere else, including elsewhere in a run, still fails.
func TestModelTemplateConcretenessExemptsOnlyTransforms(t *testing.T) {
	for name, src := range map[string]string{
		"incomplete argv":    `m: #SystemModel & {name: "m", populate: runs: [{argv: [string]}]}`,
		"incomplete set":     `m: #SystemModel & {name: "m", populate: runs: [{argv: ["true"], set: project: string}]}`,
		"incomplete timeout": `m: #SystemModel & {name: "m", populate: {timeout: string, runs: [{argv: ["true"]}]}}`,
		"incomplete name":    `m: #SystemModel & {name: string, populate: runs: [{argv: ["true"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			template, err := NewTemplate(compileTemplate(t, src, "m"), TemplateOrigin{})
			if err != nil {
				return // rejected even earlier
			}
			_, err = template.Model()
			require.Error(t, err)
		})
	}
	ok := `m: #SystemModel & {name: "m", populate: runs: [{argv: ["true"], transform: {in: _, out: in}}]}`
	template, err := NewTemplate(compileTemplate(t, ok, "m"), TemplateOrigin{})
	require.NoError(t, err)
	model, err := template.Model()
	require.NoError(t, err)
	require.NotNil(t, model.Populate.Runs[0].Transform)
}
