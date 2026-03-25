package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscover_Found(t *testing.T) {
	tmp := t.TempDir()
	pudlDir := filepath.Join(tmp, ".pudl")
	require.NoError(t, os.MkdirAll(pudlDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "workspace.cue"), []byte(`name: "myproject"`), 0644))

	ws, err := Discover(tmp)
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, tmp, ws.Root)
	assert.Equal(t, pudlDir, ws.PudlDir)
	assert.Equal(t, "myproject", ws.Name)
	assert.Equal(t, filepath.Join(pudlDir, "schema"), ws.SchemaPath)
	assert.Equal(t, filepath.Join(pudlDir, "definitions"), ws.DefinitionsPath)
}

func TestDiscover_NotFound(t *testing.T) {
	tmp := t.TempDir()

	ws, err := Discover(tmp)
	require.NoError(t, err)
	assert.Nil(t, ws)
}

func TestDiscover_WalksUp(t *testing.T) {
	tmp := t.TempDir()
	pudlDir := filepath.Join(tmp, ".pudl")
	require.NoError(t, os.MkdirAll(pudlDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "workspace.cue"), []byte(`name: "parent-project"`), 0644))

	childDir := filepath.Join(tmp, "src", "deep", "nested")
	require.NoError(t, os.MkdirAll(childDir, 0755))

	ws, err := Discover(childDir)
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, tmp, ws.Root)
	assert.Equal(t, "parent-project", ws.Name)
}

func TestDiscover_StopsAtRoot(t *testing.T) {
	// Use a temp dir that definitely has no .pudl/ ancestor
	tmp := t.TempDir()
	childDir := filepath.Join(tmp, "a", "b", "c")
	require.NoError(t, os.MkdirAll(childDir, 0755))

	ws, err := Discover(childDir)
	require.NoError(t, err)
	assert.Nil(t, ws)
}

func TestLoad_MinimalWorkspace(t *testing.T) {
	tmp := t.TempDir()
	pudlDir := filepath.Join(tmp, ".pudl")
	require.NoError(t, os.MkdirAll(pudlDir, 0755))

	// Minimal workspace.cue with no name field — should default to dir name
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "workspace.cue"), []byte(`{}`), 0644))

	ws, err := Discover(tmp)
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, filepath.Base(tmp), ws.Name)
	assert.Empty(t, ws.ToolchainMappings)
}

func TestLoad_FullWorkspace(t *testing.T) {
	tmp := t.TempDir()
	pudlDir := filepath.Join(tmp, ".pudl")
	require.NoError(t, os.MkdirAll(pudlDir, 0755))

	cueContent := `
name: "full-project"
toolchain_mappings: [
    {prefix: "aws", toolchain: "terraform"},
    {prefix: "k8s", toolchain: "kubectl"},
]
`
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "workspace.cue"), []byte(cueContent), 0644))

	ws, err := Discover(tmp)
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, "full-project", ws.Name)
	require.Len(t, ws.ToolchainMappings, 2)
	assert.Equal(t, "aws", ws.ToolchainMappings[0].Prefix)
	assert.Equal(t, "terraform", ws.ToolchainMappings[0].Toolchain)
	assert.Equal(t, "k8s", ws.ToolchainMappings[1].Prefix)
	assert.Equal(t, "kubectl", ws.ToolchainMappings[1].Toolchain)
}

func TestLoad_InvalidCUE(t *testing.T) {
	tmp := t.TempDir()
	pudlDir := filepath.Join(tmp, ".pudl")
	require.NoError(t, os.MkdirAll(pudlDir, 0755))

	// Write malformed CUE
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "workspace.cue"), []byte(`name: {{{invalid`), 0644))

	ws, err := Discover(tmp)
	assert.Error(t, err)
	assert.Nil(t, ws)
	assert.Contains(t, err.Error(), "parsing workspace.cue")
}
