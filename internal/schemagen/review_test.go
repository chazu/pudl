package schemagen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/validator"
)

func TestGeneratedSchemaStartsAsDraft(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"),
		[]byte("module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))

	result, err := NewGenerator(dir).Generate(map[string]interface{}{"id": "a", "size": 3},
		GenerateOptions{PackagePath: "gen", DefinitionName: "Thing"})
	require.NoError(t, err)
	assert.Contains(t, result.Content, `review: status: "draft"`)
	assert.NotContains(t, result.Content, "cascade_fallback", "cascade priority was removed")
	require.NoError(t, os.MkdirAll(filepath.Dir(result.FilePath), 0o755))
	require.NoError(t, os.WriteFile(result.FilePath, []byte(result.Content), 0o644))

	validator.ResetSharedLoaders()
	t.Cleanup(validator.ResetSharedLoaders)
	set := validator.LoadSchemaSet([]string{dir})
	assert.Empty(t, set.Errors)
	meta, ok := set.Metadata["gen.#Thing"]
	require.True(t, ok)
	assert.Equal(t, validator.ReviewDraft, meta.Review.Status)
	assert.False(t, meta.IsValidated())
}
