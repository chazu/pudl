package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
)

// A model that still declares value bindings fails to resolve with an error
// naming the retired field, instead of being skipped as unknown.
func TestResolveModelTemplateInRejectsRetiredBindings(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"test.models\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	modelsDir := filepath.Join(dir, "models")
	require.NoError(t, os.MkdirAll(modelsDir, 0o755))
	schema := strings.Replace(systemmodel.SchemaCUE(), "package systemmodel", "package models", 1)
	source := schema + `

#Consumer: #SystemModel & {
	name: "consumer"
	inputs: value: string
	bindings: value: {
		source: {model: "producer", schema: "resources.#Thing", identity: {name: "one"}}
		path: "/value"
	}
	populate: runs: [{argv: ["true"]}]
	desired: [{"_schema": "example.consumer", value: inputs.value}]
}
`
	require.NoError(t, os.WriteFile(filepath.Join(modelsDir, "models.cue"), []byte(source), 0o644))
	validator.ResetSharedLoaders()
	t.Cleanup(validator.ResetSharedLoaders)

	_, err := resolveModelTemplateIn(dir, "consumer")
	require.ErrorContains(t, err, "retired field")
	require.ErrorContains(t, err, "bindings")
}
