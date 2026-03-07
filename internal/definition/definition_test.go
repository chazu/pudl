package definition

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestSchemaDir copies bootstrap files into a temp directory
// mirroring the layout used by CopyBootstrapSchemas.
func setupTestSchemaDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	srcDir := filepath.Join("..", "importer", "bootstrap")
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(srcDir, path)
		targetPath := filepath.Join(tmpDir, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, content, 0644)
	})
	require.NoError(t, err)

	return tmpDir
}

func TestListDefinitions(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	definitions, err := discoverer.ListDefinitions()
	require.NoError(t, err)

	// Should find definitions from example files
	assert.GreaterOrEqual(t, len(definitions), 3, "expected at least 3 definitions")

	names := make(map[string]bool)
	for _, d := range definitions {
		names[d.Name] = true
	}
	assert.True(t, names["my_simple"], "missing my_simple definition")
	assert.True(t, names["api_endpoint"], "missing api_endpoint definition")
	assert.True(t, names["prod_instance"], "missing prod_instance definition")
}

func TestGetDefinition(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	t.Run("existing definition", func(t *testing.T) {
		def, err := discoverer.GetDefinition("my_simple")
		require.NoError(t, err)
		assert.Equal(t, "my_simple", def.Name)
		assert.Equal(t, "examples.#SimpleModel", def.ModelRef)
	})

	t.Run("non-existent definition", func(t *testing.T) {
		_, err := discoverer.GetDefinition("nonexistent")
		assert.Error(t, err)
	})
}

func TestDefinitionModelRef(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	def, err := discoverer.GetDefinition("api_endpoint")
	require.NoError(t, err)
	assert.Equal(t, "examples.#HTTPEndpointModel", def.ModelRef)
}

func TestDefinitionSocketBindings(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	def, err := discoverer.GetDefinition("prod_instance")
	require.NoError(t, err)
	assert.Equal(t, "examples.#EC2InstanceModel", def.ModelRef)

	// Should detect cross-definition reference
	assert.NotEmpty(t, def.SocketBindings, "expected socket bindings for prod_instance")
	assert.Contains(t, def.SocketBindings, "VpcId")
	assert.Equal(t, "prod_vpc.outputs.vpc_id", def.SocketBindings["VpcId"])
}

func TestDefinitionMarkerBased(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	def, err := discoverer.GetDefinition("prod_vpc")
	require.NoError(t, err)
	assert.Equal(t, "prod_vpc", def.Name)
	assert.Equal(t, "examples.#VPCModel", def.ModelRef)
}

func TestEmptyDefinitionsDir(t *testing.T) {
	tmpDir := t.TempDir()
	discoverer := NewDiscoverer(tmpDir)

	definitions, err := discoverer.ListDefinitions()
	require.NoError(t, err)
	assert.Empty(t, definitions)
}

func TestNoDefinitionsDir(t *testing.T) {
	tmpDir := t.TempDir()
	discoverer := NewDiscoverer(tmpDir)

	definitions, err := discoverer.ListDefinitions()
	require.NoError(t, err)
	assert.Nil(t, definitions)
}

func TestBuildGraphEmpty(t *testing.T) {
	g := BuildGraph(nil)
	sorted, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Nil(t, sorted)
}

func TestBuildGraphNoWiring(t *testing.T) {
	defs := []DefinitionInfo{
		{Name: "a", SocketBindings: map[string]string{}},
		{Name: "b", SocketBindings: map[string]string{}},
	}
	g := BuildGraph(defs)

	sorted, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Len(t, sorted, 2)
}

func TestBuildGraphWithWiring(t *testing.T) {
	defs := []DefinitionInfo{
		{
			Name:           "prod_instance",
			SocketBindings: map[string]string{"VpcId": "prod_vpc.outputs.vpc_id"},
		},
		{
			Name:           "prod_vpc",
			SocketBindings: map[string]string{},
		},
	}
	g := BuildGraph(defs)

	sorted, err := g.TopologicalSort()
	require.NoError(t, err)
	require.Len(t, sorted, 2)

	// prod_vpc should come before prod_instance
	vpcIdx := -1
	instanceIdx := -1
	for i, name := range sorted {
		if name == "prod_vpc" {
			vpcIdx = i
		}
		if name == "prod_instance" {
			instanceIdx = i
		}
	}
	assert.Less(t, vpcIdx, instanceIdx, "prod_vpc should come before prod_instance")
}

func TestBuildGraphDependencies(t *testing.T) {
	defs := []DefinitionInfo{
		{
			Name:           "prod_instance",
			SocketBindings: map[string]string{"VpcId": "prod_vpc.outputs.vpc_id"},
		},
		{
			Name:           "prod_vpc",
			SocketBindings: map[string]string{},
		},
	}
	g := BuildGraph(defs)

	deps := g.GetDependencies("prod_instance")
	assert.Equal(t, []string{"prod_vpc"}, deps)

	deps = g.GetDependencies("prod_vpc")
	assert.Nil(t, deps)

	dependents := g.GetDependents("prod_vpc")
	assert.Equal(t, []string{"prod_instance"}, dependents)
}

func TestBuildGraphCycleDetection(t *testing.T) {
	defs := []DefinitionInfo{
		{
			Name:           "a",
			SocketBindings: map[string]string{"x": "b.outputs.y"},
		},
		{
			Name:           "b",
			SocketBindings: map[string]string{"y": "a.outputs.x"},
		},
	}
	g := BuildGraph(defs)

	_, err := g.TopologicalSort()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle detected")
}

func TestExtractReferencedDef(t *testing.T) {
	assert.Equal(t, "prod_vpc", extractReferencedDef("prod_vpc.outputs.vpc_id"))
	assert.Equal(t, "foo", extractReferencedDef("foo.schema.bar"))
	assert.Equal(t, "", extractReferencedDef("noDot"))
}
