package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/validator"
	"github.com/chazu/pudl/internal/workspace"
)

func TestModelValidateAcceptsStructurallyValidBoundTemplate(t *testing.T) {
	schemaRoot := filepath.Join(t.TempDir(), "schema")
	writeRunSetFixtureSchemas(t, schemaRoot)
	validator.ResetSharedLoaders()
	t.Cleanup(validator.ResetSharedLoaders)

	previousPolicy := wsPolicy
	wsPolicy = &workspace.Policy{ModelSearchPaths: []string{schemaRoot}}
	t.Cleanup(func() { wsPolicy = previousPolicy })

	require.NoError(t, modelValidateCmd.RunE(modelValidateCmd, []string{"consumer"}))
}

func TestModelValidateRejectsDesiredAtSensitivePath(t *testing.T) {
	schemaRoot := filepath.Join(t.TempDir(), "schema")
	writeRunSetFixtureSchemas(t, schemaRoot)
	require.NoError(t, os.WriteFile(filepath.Join(schemaRoot, "pudl", "resources", "secret.cue"), []byte(`package resources

#Secret: {
	_pudl: {schema_type: "base", resource_type: "resources.secret", identity_fields: ["name"], sensitive_fields: ["token"]}
	name: string
	...
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(schemaRoot, "pudl", "systemmodel", "leaky.cue"), []byte(`package systemmodel

#Leaky: #SystemModel & {
	name: "leaky"
	populate: runs: [{argv: ["true"]}]
	desired: [{"_schema": "resources.secret", name: "ci", token: "plaintext"}]
}
`), 0o644))
	validator.ResetSharedLoaders()
	inference.ResetShared()
	t.Cleanup(func() { validator.ResetSharedLoaders(); inference.ResetShared() })
	previousPolicy := wsPolicy
	wsPolicy = &workspace.Policy{ModelSearchPaths: []string{schemaRoot}, SchemaSearchPaths: []string{schemaRoot}}
	t.Cleanup(func() { wsPolicy = previousPolicy })

	template, _, _, err := resolveModelTemplate("leaky")
	require.NoError(t, err)
	problems, err := validateModelTemplate(template)
	require.NoError(t, err)
	require.Len(t, problems, 1)
	require.Contains(t, problems[0], `model "leaky": desired[0] (resources.secret) sets sensitive path "token"`)
}
