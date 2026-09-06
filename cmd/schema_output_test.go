package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
	"github.com/chazu/pudl/internal/workspace"
)

func schemaOutputWorkspace(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".pudl", "schema")
	writeSchemaOutputFile(t, root, "cue.mod/module.cue", "module: \"pudl.schemas@v0\"\nlanguage: version: \"v0.14.0\"\n")
	previous := wsPolicy
	wsPolicy = &workspace.Policy{SchemaSearchPaths: []string{root}, ModelSearchPaths: []string{root}}
	t.Cleanup(func() { wsPolicy = previous })
	validator.ResetSharedLoaders()
	t.Cleanup(validator.ResetSharedLoaders)
	return root
}

func writeSchemaOutputFile(t *testing.T, root, path, content string) {
	t.Helper()
	path = filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestModelShowSchemaReferences(t *testing.T) {
	root := schemaOutputWorkspace(t)
	writeSchemaOutputFile(t, root, "user/resources/types.cue", `package resources
#Project: {
	_pudl: {schema_type: "base", resource_type: "custom.project"}
	name: string
}
#First: {
	_pudl: {schema_type: "base", resource_type: "custom.shared"}
	name: string
}
#Second: {
	_pudl: {schema_type: "base", resource_type: "custom.shared"}
	name: string
}
`)
	model := strings.Replace(systemmodel.SchemaCUE(), "package systemmodel", "package models", 1) + `
#Demo: #SystemModel & {
	name: "demo"
	populate: {plugin: "demo", differential: false}
	desired: [
		{"_schema": "custom.project", name: "one"},
		{"_schema": "user/resources.#Project", name: "two"},
		{"_schema": "pudl.schemas/user/resources@v0:#Project", name: "three"},
		{"_schema": "custom.shared", name: "four"},
		{"_schema": "missing.type", name: "five"},
		{"_schema": "user/resources:#Missing", name: "six"},
	]
}
`
	writeSchemaOutputFile(t, root, "models/model.cue", model)
	previousJSON := modelShowJSON
	modelShowJSON = false
	t.Cleanup(func() { modelShowJSON = previousJSON })
	output := captureQueryOutput(t, func() error { return modelShowCmd.RunE(modelShowCmd, []string{"demo"}) })
	require.Contains(t, output, "Desired:   6 resource(s)")
	require.Contains(t, output, "user/resources.#Project (resource type: custom.project)")
	require.Equal(t, 2, strings.Count(output, "    - user/resources.#Project\n"))
	require.Contains(t, output, "resource type: custom.shared (multiple schemas: user/resources.#First, user/resources.#Second)")
	require.Contains(t, output, "resource type: missing.type (no registered schema)")
	require.Contains(t, output, "user/resources.#Missing (not registered)")
	require.NotContains(t, output, "missing.#type")
	require.NotContains(t, output, "pudl.schemas/")
	for _, ref := range []string{"user/resources.#Project", "user/resources.#First", "user/resources.#Second"} {
		shown := captureQueryOutput(t, func() error { return runSchemaShowCommand(ref) })
		require.Contains(t, shown, "Schema: "+ref)
	}

	// Schema display must not rewrite the model's routing tags or JSON payload.
	modelShowJSON = true
	output = captureQueryOutput(t, func() error { return modelShowCmd.RunE(modelShowCmd, []string{"demo"}) })
	var decoded systemmodel.SystemModel
	require.NoError(t, json.Unmarshal([]byte(output), &decoded))
	require.Equal(t, "custom.project", decoded.Desired[0]["_schema"])
	require.Equal(t, "pudl.schemas/user/resources@v0:#Project", decoded.Desired[2]["_schema"])
}

func TestModelShowSchemaReferencesHonorWorkspaceShadowing(t *testing.T) {
	root := schemaOutputWorkspace(t)
	global := t.TempDir()
	writeSchemaOutputFile(t, global, "cue.mod/module.cue", "module: \"pudl.schemas@v0\"\nlanguage: version: \"v0.14.0\"\n")
	for path, resourceType := range map[string]string{root: "local.project", global: "global.project"} {
		writeSchemaOutputFile(t, path, "user/resources/types.cue", `package resources
#Project: {
	_pudl: {schema_type: "base", resource_type: "`+resourceType+`"}
	name: string
}
`)
	}
	schemas, err := validator.NewChainValidator(root, global)
	require.NoError(t, err)
	require.Equal(t, "user/resources.#Project (resource type: local.project)", describeDesiredSchema("local.project", schemas))
	require.Equal(t, "resource type: global.project (no registered schema)", describeDesiredSchema("global.project", schemas))
}

func TestSchemaListReferencesCanBeShown(t *testing.T) {
	root := schemaOutputWorkspace(t)
	writeSchemaOutputFile(t, root, "user/resources/types.cue", "package resources\n#First: {name: string}\n#Second: {id: int}\n")
	previousVerbose, previousPackage := schemaVerbose, schemaPackage
	t.Cleanup(func() { schemaVerbose, schemaPackage = previousVerbose, previousPackage })
	for _, schemaVerbose = range []bool{false, true} {
		for _, schemaPackage = range []string{"", "user/resources"} {
			output := captureQueryOutput(t, runSchemaListCommand)
			references := regexp.MustCompile(`user/resources\.#[A-Za-z]+`).FindAllString(output, -1)
			require.Equal(t, []string{"user/resources.#First", "user/resources.#Second"}, references)
			for _, ref := range references {
				shown := captureQueryOutput(t, func() error { return runSchemaShowCommand(ref) })
				require.Contains(t, shown, "Schema: "+ref)
			}
		}
	}
}

func TestSchemaAddPrintsDefinitionsInsteadOfFileSlug(t *testing.T) {
	schemaOutputWorkspace(t)
	source := filepath.Join(t.TempDir(), "input.cue")
	require.NoError(t, os.WriteFile(source, []byte("package custom\n#ActualShape: {name: string}\n"), 0o644))
	output := captureQueryOutput(t, func() error { return runSchemaAddCommand([]string{"custom.file-slug", source}) })
	require.Contains(t, output, "Definitions: custom.#ActualShape")
	require.Contains(t, output, "pudl schema show custom.#ActualShape")
	require.Contains(t, output, "--schema custom.#ActualShape")
	require.NotContains(t, output, "--schema custom.file-slug")
	shown := captureQueryOutput(t, func() error { return runSchemaShowCommand("custom.#ActualShape") })
	require.Contains(t, shown, "#ActualShape:")
}
