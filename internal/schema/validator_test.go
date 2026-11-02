package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pudl metadata lives in _pudl; the validator must not ask for the legacy
// _identity/_tracked/_version fields.
func TestValidateSchemaNoLegacyMetadataWarnings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thing.cue")
	require.NoError(t, os.WriteFile(path, []byte(`package custom

#Thing: {
	_pudl: {
		schema_type:     "base"
		identity_fields: ["id"]
	}
	id: string
}
`), 0o644))

	result, err := NewValidator().ValidateSchema(path)
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Warnings)
	assert.Equal(t, "custom", result.PackageName)
	assert.Equal(t, []string{"#Thing"}, result.Definitions)
}
