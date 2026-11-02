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
		"plugin observe": {field: "populate.plugin", source: `
app: #SystemModel & {
	name: "app"
	populate: #PluginObserve & {plugin: "k8s", input: {namespace: "default"}}
}`},
		"ewe target": {field: "populate.eweSource", source: `
app: #SystemModel & {
	name: "app"
	populate: #EweTarget & {eweSource: "p.cue", outputs: ["r.json"]}
}`},
		"plugins block": {field: "plugins", source: `
app: #SystemModel & {
	name: "app"
	plugins: [{name: "k8s", script: "./plugin.bb"}]
	populate: {runs: [{argv: ["true"]}]}
}`},
		"value bindings": {field: "bindings", source: `
app: #SystemModel & {
	name: "app"
	inputs: value: string
	bindings: value: {
		source: {model: "producer", schema: "a.#B", identity: {name: "one"}}
		path: "/value"
	}
	populate: {runs: [{argv: ["echo", inputs.value]}]}
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
