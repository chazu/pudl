package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
	"github.com/chazu/pudl/internal/wiring"
)

func TestResolveModelTemplateInRetainsIncompleteBindingModel(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"test.models\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	modelsDir := filepath.Join(dir, "models")
	require.NoError(t, os.MkdirAll(modelsDir, 0o755))
	schema := strings.Replace(systemmodel.SchemaCUE(), "package systemmodel", "package models", 1)
	source := schema + `

#Consumer: #SystemModel & {
	name: "consumer"
	inputs: value: string @pudl(binding=plain)
	bindings: value: {
		source: {model: "producer", schema: "resources.#Thing", identity: {name: "one"}}
		path: "/value"
	}
	populate: #PluginObserve & {plugin: "host", differential: false}
	desired: [{"_schema": "example.consumer", value: inputs.value}]
}
`
	require.NoError(t, os.WriteFile(filepath.Join(modelsDir, "models.cue"), []byte(source), 0o644))
	validator.ResetSharedLoaders()
	t.Cleanup(validator.ResetSharedLoaders)

	template, err := resolveModelTemplateIn(dir, "consumer")
	require.NoError(t, err)
	require.NotNil(t, template)
	assert.Equal(t, "consumer", template.Name)
	assert.Equal(t, "models.#Consumer", template.Origin.SchemaName)
	assert.Equal(t, modelsDir, template.Origin.LoadDir)
	require.Contains(t, template.Bindings, "value")
	models, err := listModelsIn(dir)
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Nil(t, models[0].Model)
	assert.Equal(t, 1, models[0].Summary.DesiredCount)

	_, err = template.Elaborate(map[string]any{})
	require.ErrorContains(t, err, "missing: value")

	diagnostic := resolutionDiagnosticReport(template, runFlags{dryRun: true}, &wiring.ResolutionError{
		Input: "value",
		Code:  "source-absent",
		Cause: errors.New("no matching resource"),
	})
	assert.Equal(t, "consumer", diagnostic.Model)
	assert.Equal(t, "dry-run", diagnostic.Mode)
	assert.Equal(t, "failed", diagnostic.CompletionStatus)
	assert.False(t, diagnostic.OK)
	require.Len(t, diagnostic.BindingIssues, 1)
	assert.Equal(t, wiring.BindingIssue{
		Input:         "value",
		ProducerModel: "producer",
		Schema:        "resources.#Thing",
		Identity:      map[string]any{"name": "one"},
		Path:          "/value",
		Code:          "source-absent",
		Message:       `resolve binding "value": source-absent: no matching resource`,
	}, diagnostic.BindingIssues[0])
}

func TestResolveModelTemplateInReportsRequestedInvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"test.models\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	modelsDir := filepath.Join(dir, "models")
	require.NoError(t, os.MkdirAll(modelsDir, 0o755))
	schema := strings.Replace(systemmodel.SchemaCUE(), "package systemmodel", "package models", 1)
	source := schema + `

#InvalidPointer: #SystemModel & {
	name: "invalid-pointer"
	inputs: value: string @pudl(binding=plain)
	bindings: value: {
		source: {model: "producer", schema: "resources.#Thing", identity: {name: "one"}}
		path: "value"
	}
	populate: #PluginObserve & {plugin: "host", differential: false}
}
`
	require.NoError(t, os.WriteFile(filepath.Join(modelsDir, "models.cue"), []byte(source), 0o644))
	validator.ResetSharedLoaders()
	t.Cleanup(validator.ResetSharedLoaders)

	_, err := resolveModelTemplateIn(dir, "invalid-pointer")
	require.ErrorContains(t, err, "RFC 6901 JSON Pointer")
}
