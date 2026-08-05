package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

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
