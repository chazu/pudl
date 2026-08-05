package systemmodel

import (
	"encoding/json"
	"fmt"
	"strings"
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

func plainTemplateSource(slot string) string {
	return fmt.Sprintf(`
app: #SystemModel & {
	name: "app"
	inputs: {subnet_id: %s}
	bindings: subnet_id: {
		source: {model: "network", schema: "pudl/aws.#Subnet", identity: {SubnetId: "private"}}
		path: "/SubnetId"
	}
	populate: #PluginObserve & {plugin: "host", differential: false}
	desired: [{"_schema": "example.application", name: "api", subnet_id: inputs.subnet_id}]
}`, slot)
}

func TestModelTemplateElaboratesPlainInput(t *testing.T) {
	value := compileTemplate(t, plainTemplateSource(`string @pudl(binding=plain)`), "app")
	template, err := NewTemplate(value, TemplateOrigin{SchemaName: "models.#App", LoadDir: "/models"})
	require.NoError(t, err)
	assert.Equal(t, "app", template.Name)
	assert.Equal(t, "/models", template.Origin.LoadDir)
	assert.Same(t, value.Context(), template.Context())
	require.Contains(t, template.Inputs, "subnet_id")
	require.Contains(t, template.Bindings, "subnet_id")

	model, err := template.Elaborate(map[string]any{"subnet_id": "subnet-123"})
	require.NoError(t, err)
	require.Len(t, model.Desired, 1)
	assert.Equal(t, "subnet-123", model.Desired[0]["subnet_id"])

	encoded, err := json.Marshal(model)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "bindings")
	assert.NotContains(t, string(encoded), "inputs")
}

func TestModelTemplateRejectsWrongPlainValueType(t *testing.T) {
	template, err := NewTemplate(compileTemplate(t, plainTemplateSource(`string @pudl(binding=plain)`), "app"), TemplateOrigin{})
	require.NoError(t, err)

	_, err = template.Elaborate(map[string]any{"subnet_id": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting values")
}

func TestModelTemplateRequiresExactInputSet(t *testing.T) {
	template, err := NewTemplate(compileTemplate(t, plainTemplateSource(`string @pudl(binding=plain)`), "app"), TemplateOrigin{})
	require.NoError(t, err)

	_, err = template.Elaborate(nil)
	require.ErrorContains(t, err, "missing: subnet_id")
	_, err = template.Elaborate(map[string]any{"subnet_id": "ok", "extra": true})
	require.ErrorContains(t, err, "extra: extra")
}

func TestModelTemplateInputContractFailures(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "unannotated", src: plainTemplateSource("string"), want: "binding=plain"},
		{name: "optional", src: strings.Replace(plainTemplateSource(`string @pudl(binding=plain)`), `inputs: {subnet_id: string @pudl(binding=plain)}`, `inputs: {subnet_id?: string @pudl(binding=plain)}`, 1), want: "must be required"},
		{name: "nested", src: plainTemplateSource(`{value: string} @pudl(binding=plain)`), want: "scalar constraint"},
		{name: "invalid pointer", src: strings.Replace(plainTemplateSource(`string @pudl(binding=plain)`), `path: "/SubnetId"`, `path: "SubnetId"`, 1), want: "JSON Pointer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := cuecontext.New()
			root := ctx.CompileString(schemaCUE+"\n"+tt.src, cue.Filename("template.cue"))
			if root.Err() != nil {
				assert.Contains(t, "compile", tt.want)
				return
			}
			_, err := NewTemplate(root.LookupPath(cue.ParsePath("app")), TemplateOrigin{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestModelTemplateHonorsInheritedPlainAnnotation(t *testing.T) {
	const source = `
#PlainSubnet: string @pudl(binding=plain)
app: #SystemModel & {
	name: "app"
	inputs: subnet_id: #PlainSubnet
	bindings: subnet_id: {
		source: {model: "network", schema: "pudl/aws.#Subnet", identity: {SubnetId: "private"}}
		path: "/SubnetId"
	}
	populate: #PluginObserve & {plugin: "host", differential: false}
}`
	_, err := NewTemplate(compileTemplate(t, source, "app"), TemplateOrigin{})
	require.NoError(t, err)
}

func TestModelTemplateRejectsConflictingInheritedAnnotations(t *testing.T) {
	const source = `
#Plain: string @pudl(binding=plain)
#Sealed: string @pudl(binding=sealed)
app: #SystemModel & {
	name: "app"
	inputs: token: #Plain & #Sealed
	bindings: token: {
		source: {model: "producer", schema: "example.#Thing", identity: {name: "one"}}
		path: "/name"
	}
	populate: #PluginObserve & {plugin: "host", differential: false}
}`
	_, err := NewTemplate(compileTemplate(t, source, "app"), TemplateOrigin{})
	require.ErrorContains(t, err, "conflicting")
}

func TestModelTemplateDecodesAndRedactsSealedDeclarations(t *testing.T) {
	const source = `
app: #SystemModel & {
	name: "app"
	populate: #PluginObserve & {
		plugin: "host"
		differential: false
		sealed_inputs: API_TOKEN: {
			ref: "env:API_TOKEN"
			delivery_mode: "env"
		} @pudl(binding=sealed)
	}
	converge: #PluginPlan & {
		plugin: "host"
		sealed_inputs: GENERATED: {
			source: {model: "producer", output: "TOKEN"}
			delivery_mode: "file"
		} @pudl(binding=sealed)
		sealed_outputs: LOCAL: {
			ref: "pass:services/generated"
			store_mode: "create_if_absent"
		} @pudl(binding=sealed)
	}
}`
	template, err := NewTemplate(compileTemplate(t, source, "app"), TemplateOrigin{})
	require.NoError(t, err)
	assert.True(t, template.HasSealedOutputs())
	model, err := template.Elaborate(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "env:API_TOKEN", model.Populate.SealedInputs["API_TOKEN"].Ref)
	assert.Equal(t, "create_if_absent", model.Converge.SealedOutputs["LOCAL"].StoreMode)
	require.NotNil(t, model.Converge.SealedInputs["GENERATED"].Source)
	assert.Equal(t, "producer", model.Converge.SealedInputs["GENERATED"].Source.Model)

	encoded, err := json.Marshal(model)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "env:API_TOKEN")
	assert.NotContains(t, string(encoded), "pass:services/generated")
}

func TestModelTemplateSealedContractFailures(t *testing.T) {
	const unannotated = `
app: #SystemModel & {
	name: "app"
	populate: #PluginObserve & {
		plugin: "host"
		sealed_inputs: TOKEN: {ref: "env:TOKEN", delivery_mode: "env"}
	}
}`
	_, err := NewTemplate(compileTemplate(t, unannotated, "app"), TemplateOrigin{})
	require.ErrorContains(t, err, "binding=sealed")

}

func TestSystemModelSchemaRejectsPopulateSealedOutputs(t *testing.T) {
	tests := map[string]string{
		"plugin observe": `
app: #SystemModel & {
	name: "app"
	populate: #PluginObserve & {
		plugin: "host"
		sealed_outputs: TOKEN: {ref: "env:A", store_mode: "create"} @pudl(binding=sealed)
	}
}`,
		"ewe": `
app: #SystemModel & {
	name: "app"
	populate: #EweTarget & {
		eweSource: "populate.cue"
		outputs: ["records.json"]
		sealed_outputs: TOKEN: {ref: "env:A", store_mode: "create"} @pudl(binding=sealed)
	}
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			root := cuecontext.New().CompileString(schemaCUE+"\n"+source, cue.Filename("template.cue"))
			require.Error(t, root.Err(), "populate sealed outputs must be rejected structurally")
		})
	}
}

func TestModelTemplateCollectsPlainAndSealedDependencyProducers(t *testing.T) {
	source := strings.Replace(plainTemplateSource(`string @pudl(binding=plain)`),
		`populate: #PluginObserve & {plugin: "host", differential: false}`,
		`populate: #PluginObserve & {
			plugin: "host", differential: false
			sealed_inputs: TOKEN: {
				source: {model: "secrets", output: "TOKEN"}
				delivery_mode: "env"
			} @pudl(binding=sealed)
			sealed_inputs: DIRECT: {
				ref: "env:DIRECT_TOKEN"
				delivery_mode: "env"
			} @pudl(binding=sealed)
		}`, 1)
	template, err := NewTemplate(compileTemplate(t, source, "app"), TemplateOrigin{})
	require.NoError(t, err)
	assert.Equal(t, []string{"network", "secrets"}, template.BindingProducers)
}

func TestModelTemplateRejectsSelfBinding(t *testing.T) {
	source := strings.Replace(plainTemplateSource(`string @pudl(binding=plain)`),
		`model: "network"`, `model: "app"`, 1)
	_, err := NewTemplate(compileTemplate(t, source, "app"), TemplateOrigin{})
	require.ErrorContains(t, err, "cannot bind to itself")
}

func TestModelTemplateCannotDeclareWorkspaceSecretWritePolicy(t *testing.T) {
	source := strings.Replace(plainTemplateSource(`string @pudl(binding=plain)`),
		`name: "app"`, `name: "app"
	secrets: writable_refs: ["pass:untrusted/*"]`, 1)
	value := cuecontext.New().CompileString(schemaCUE+"\n"+source, cue.Filename("template.cue"))
	err := value.Err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secrets")
}
