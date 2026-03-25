package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContext_WithWorkspace(t *testing.T) {
	tmp := t.TempDir()
	pudlDir := filepath.Join(tmp, ".pudl")
	require.NoError(t, os.MkdirAll(pudlDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "workspace.cue"), []byte(`name: "ctx-project"`), 0644))

	ws := &Workspace{
		Root:            tmp,
		PudlDir:         pudlDir,
		Name:            "ctx-project",
		SchemaPath:      filepath.Join(pudlDir, "schema"),
		DefinitionsPath: filepath.Join(pudlDir, "definitions"),
	}

	globalDir := filepath.Join(tmp, "global-pudl")
	ctx := buildContext(ws, globalDir)

	assert.NotNil(t, ctx.Workspace)
	assert.Equal(t, "ctx-project", ctx.EffectiveOrigin)
	assert.Equal(t, globalDir, ctx.GlobalPudlDir)

	// Per-repo schema path should come first, then global
	require.Len(t, ctx.SchemaSearchPaths, 2)
	assert.Equal(t, filepath.Join(pudlDir, "schema"), ctx.SchemaSearchPaths[0])
	assert.Equal(t, filepath.Join(globalDir, "schema"), ctx.SchemaSearchPaths[1])

	// Per-repo definitions path should come first, then global
	require.Len(t, ctx.DefinitionSearchPaths, 2)
	assert.Equal(t, filepath.Join(pudlDir, "definitions"), ctx.DefinitionSearchPaths[0])
	assert.Equal(t, filepath.Join(globalDir, "schema", "definitions"), ctx.DefinitionSearchPaths[1])
}

func TestNewContext_GlobalOnly(t *testing.T) {
	globalDir := filepath.Join(t.TempDir(), "global-pudl")

	ctx := buildContext(nil, globalDir)

	assert.Nil(t, ctx.Workspace)
	assert.Equal(t, "global", ctx.EffectiveOrigin)
	assert.Equal(t, globalDir, ctx.GlobalPudlDir)

	// Only global schema path
	require.Len(t, ctx.SchemaSearchPaths, 1)
	assert.Equal(t, filepath.Join(globalDir, "schema"), ctx.SchemaSearchPaths[0])

	// Only global definitions path
	require.Len(t, ctx.DefinitionSearchPaths, 1)
	assert.Equal(t, filepath.Join(globalDir, "schema", "definitions"), ctx.DefinitionSearchPaths[0])
}
