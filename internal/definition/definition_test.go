package definition

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDefinitionsDir creates a temp schema dir with test definition files.
func setupTestDefinitionsDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	defsDir := filepath.Join(tmpDir, "definitions")
	require.NoError(t, os.MkdirAll(defsDir, 0755))

	// Schema unification pattern definition
	unifyDef := `package definitions

prod_instance: examples.#EC2Instance & {
	VpcId: prod_vpc.outputs.vpc_id
}
`
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "instance.cue"), []byte(unifyDef), 0644))

	// Marker-based definition
	markerDef := `package definitions

prod_vpc: {
	_schema: "examples.#VPC"
	cidr: "10.0.0.0/16"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "vpc.cue"), []byte(markerDef), 0644))

	return tmpDir
}

func TestListDefinitions(t *testing.T) {
	schemaDir := setupTestDefinitionsDir(t)
	discoverer := NewDiscoverer(schemaDir)

	definitions, err := discoverer.ListDefinitions()
	require.NoError(t, err)

	assert.Len(t, definitions, 2)

	names := make(map[string]bool)
	for _, d := range definitions {
		names[d.Name] = true
	}
	assert.True(t, names["prod_instance"], "missing prod_instance definition")
	assert.True(t, names["prod_vpc"], "missing prod_vpc definition")
}

func TestGetDefinition(t *testing.T) {
	schemaDir := setupTestDefinitionsDir(t)
	discoverer := NewDiscoverer(schemaDir)

	t.Run("existing definition", func(t *testing.T) {
		def, err := discoverer.GetDefinition("prod_instance")
		require.NoError(t, err)
		assert.Equal(t, "prod_instance", def.Name)
		assert.Equal(t, "examples.#EC2Instance", def.SchemaRef)
	})

	t.Run("non-existent definition", func(t *testing.T) {
		_, err := discoverer.GetDefinition("nonexistent")
		assert.Error(t, err)
	})
}

func TestDefinitionSchemaRef(t *testing.T) {
	schemaDir := setupTestDefinitionsDir(t)
	discoverer := NewDiscoverer(schemaDir)

	def, err := discoverer.GetDefinition("prod_instance")
	require.NoError(t, err)
	assert.Equal(t, "examples.#EC2Instance", def.SchemaRef)
}

func TestDefinitionSocketBindings(t *testing.T) {
	schemaDir := setupTestDefinitionsDir(t)
	discoverer := NewDiscoverer(schemaDir)

	def, err := discoverer.GetDefinition("prod_instance")
	require.NoError(t, err)

	assert.NotEmpty(t, def.SocketBindings, "expected socket bindings for prod_instance")
	assert.Contains(t, def.SocketBindings, "VpcId")
	assert.Equal(t, "prod_vpc.outputs.vpc_id", def.SocketBindings["VpcId"])
}

func TestDefinitionMarkerBased(t *testing.T) {
	schemaDir := setupTestDefinitionsDir(t)
	discoverer := NewDiscoverer(schemaDir)

	def, err := discoverer.GetDefinition("prod_vpc")
	require.NoError(t, err)
	assert.Equal(t, "prod_vpc", def.Name)
	assert.Equal(t, "examples.#VPC", def.SchemaRef)
}

func TestEmptyDefinitionsDir(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "definitions"), 0755)
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

// --- Multi-path discovery tests ---

// setupMultiPathDirs creates two schema dirs: "per-repo" and "global" with definitions.
func setupMultiPathDirs(t *testing.T, perRepoDefs map[string]string, globalDefs map[string]string) (perRepo string, global string) {
	t.Helper()

	perRepo = t.TempDir()
	global = t.TempDir()

	if perRepoDefs != nil {
		defsDir := filepath.Join(perRepo, "definitions")
		require.NoError(t, os.MkdirAll(defsDir, 0755))
		for name, content := range perRepoDefs {
			require.NoError(t, os.WriteFile(filepath.Join(defsDir, name), []byte(content), 0644))
		}
	}

	if globalDefs != nil {
		defsDir := filepath.Join(global, "definitions")
		require.NoError(t, os.MkdirAll(defsDir, 0755))
		for name, content := range globalDefs {
			require.NoError(t, os.WriteFile(filepath.Join(defsDir, name), []byte(content), 0644))
		}
	}

	return perRepo, global
}

func TestMultiDiscoverer_PerRepoFirst(t *testing.T) {
	// Both dirs define "app_server" — per-repo version should win.
	perRepoDef := `package definitions

app_server: local.#AppServer & {
	port: 8080
}
`
	globalDef := `package definitions

app_server: global.#AppServer & {
	port: 9090
}
`
	perRepo, global := setupMultiPathDirs(t,
		map[string]string{"app.cue": perRepoDef},
		map[string]string{"app.cue": globalDef},
	)

	disc := NewMultiDiscoverer([]string{perRepo, global})
	defs, err := disc.ListDefinitions()
	require.NoError(t, err)
	require.Len(t, defs, 1)
	assert.Equal(t, "app_server", defs[0].Name)
	assert.Equal(t, "local.#AppServer", defs[0].SchemaRef, "per-repo definition should shadow global")
}

func TestMultiDiscoverer_MergesBoth(t *testing.T) {
	// Per-repo has app_server, global has database — both should appear.
	perRepoDef := `package definitions

app_server: examples.#AppServer & {
	port: 8080
}
`
	globalDef := `package definitions

database: examples.#Database & {
	engine: "postgres"
}
`
	perRepo, global := setupMultiPathDirs(t,
		map[string]string{"app.cue": perRepoDef},
		map[string]string{"db.cue": globalDef},
	)

	disc := NewMultiDiscoverer([]string{perRepo, global})
	defs, err := disc.ListDefinitions()
	require.NoError(t, err)
	require.Len(t, defs, 2)

	names := make(map[string]bool)
	for _, d := range defs {
		names[d.Name] = true
	}
	assert.True(t, names["app_server"], "missing app_server from per-repo")
	assert.True(t, names["database"], "missing database from global")
}

func TestMultiDiscoverer_EmptyPerRepo(t *testing.T) {
	// Per-repo definitions dir is empty, global has definitions.
	globalDef := `package definitions

database: examples.#Database & {
	engine: "postgres"
}
`
	perRepo, global := setupMultiPathDirs(t,
		map[string]string{}, // empty per-repo
		map[string]string{"db.cue": globalDef},
	)

	disc := NewMultiDiscoverer([]string{perRepo, global})
	defs, err := disc.ListDefinitions()
	require.NoError(t, err)
	require.Len(t, defs, 1)
	assert.Equal(t, "database", defs[0].Name)
}

func TestMultiDiscoverer_NoDirs(t *testing.T) {
	// All paths point to non-existent directories.
	disc := NewMultiDiscoverer([]string{"/tmp/nonexistent-path-1", "/tmp/nonexistent-path-2"})
	defs, err := disc.ListDefinitions()
	require.NoError(t, err)
	assert.Empty(t, defs)
}

func TestNewDiscoverer_BackwardCompat(t *testing.T) {
	// Single-path NewDiscoverer should work identically to before.
	schemaDir := setupTestDefinitionsDir(t)
	disc := NewDiscoverer(schemaDir)

	defs, err := disc.ListDefinitions()
	require.NoError(t, err)
	assert.Len(t, defs, 2)

	def, err := disc.GetDefinition("prod_instance")
	require.NoError(t, err)
	assert.Equal(t, "examples.#EC2Instance", def.SchemaRef)
}
