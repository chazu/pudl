package systemmodel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A model that still uses a retired field must fail to load with an error
// naming the field and the migration notes, through both load paths.
func TestRetiredFieldsFailToLoadWithActionableError(t *testing.T) {
	cases := map[string]struct {
		source string
		field  string
	}{
		"converge": {field: "converge", source: `
app: #SystemModel & {
	name: "app"
	populate: {runs: [{argv: ["true"]}]}
	desired: [{"_schema": "a.#B", name: "n"}]
	converge: #PluginPlan & {plugin: "k8s", input: {namespace: "default"}}
}`},
		"sealed outputs": {field: "converge.sealed_outputs", source: `
app: #SystemModel & {
	name: "app"
	populate: {runs: [{argv: ["true"]}]}
	converge: {plugin: "k8s", sealed_outputs: TOKEN: {ref: "pass:a", store_mode: "create"}}
}`},
		"sealed inputs": {field: "populate.sealed_inputs", source: `
app: #SystemModel & {
	name: "app"
	populate: #EweTarget & {
		eweSource: "p.cue"
		outputs: ["r.json"]
		sealed_inputs: TOKEN: {ref: "env:TOKEN", delivery_mode: "env"}
	}
}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			value := compileTemplate(t, tc.source, "app")
			_, err := NewTemplate(value, TemplateOrigin{})
			require.ErrorContains(t, err, tc.field)
			require.ErrorContains(t, err, RetiredFieldsDoc)

			_, err = DecodeValue(value)
			require.ErrorContains(t, err, tc.field)
			require.ErrorContains(t, err, RetiredFieldsDoc)
		})
	}
}
