package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestSchemaDir copies example model CUE files into a temp directory
// with the same layout as the bootstrap schema path.
func setupTestSchemaDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	// Read bootstrap example files from the actual source tree
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

func TestListModels(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	models, err := discoverer.ListModels()
	require.NoError(t, err)

	// Should find all 3 example models
	assert.GreaterOrEqual(t, len(models), 3, "expected at least 3 models")

	// Check model names exist
	names := make(map[string]bool)
	for _, m := range models {
		names[m.Name] = true
	}
	assert.True(t, names["examples.#EC2InstanceModel"], "missing EC2 model")
	assert.True(t, names["examples.#HTTPEndpointModel"], "missing HTTP model")
	assert.True(t, names["examples.#SimpleModel"], "missing Simple model")
}

func TestGetModel(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	t.Run("existing model", func(t *testing.T) {
		m, err := discoverer.GetModel("examples.#EC2InstanceModel")
		require.NoError(t, err)
		assert.Equal(t, "ec2_instance", m.Metadata.Name)
		assert.Equal(t, "compute", m.Metadata.Category)
	})

	t.Run("non-existent model", func(t *testing.T) {
		_, err := discoverer.GetModel("nonexistent.#Model")
		assert.Error(t, err)
	})
}

func TestEC2ModelParsing(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	m, err := discoverer.GetModel("examples.#EC2InstanceModel")
	require.NoError(t, err)

	// Metadata
	assert.Equal(t, "ec2_instance", m.Metadata.Name)
	assert.Equal(t, "AWS EC2 compute instance", m.Metadata.Description)
	assert.Equal(t, "compute", m.Metadata.Category)
	assert.Equal(t, "server", m.Metadata.Icon)

	// Methods
	assert.Len(t, m.Methods, 5)
	assert.Equal(t, "action", m.Methods["list"].Kind)
	assert.Equal(t, "action", m.Methods["create"].Kind)
	assert.Equal(t, "action", m.Methods["delete"].Kind)
	assert.Equal(t, "qualification", m.Methods["valid_credentials"].Kind)
	assert.Equal(t, "qualification", m.Methods["ami_exists"].Kind)

	// Qualification blocks
	assert.Contains(t, m.Methods["valid_credentials"].Blocks, "create")
	assert.Contains(t, m.Methods["valid_credentials"].Blocks, "delete")
	assert.Contains(t, m.Methods["valid_credentials"].Blocks, "list")
	assert.Contains(t, m.Methods["ami_exists"].Blocks, "create")

	// Sockets
	assert.Len(t, m.Sockets, 4)
	assert.Equal(t, "input", m.Sockets["vpc_id"].Direction)
	assert.Equal(t, "output", m.Sockets["instance_id"].Direction)
	assert.Equal(t, false, m.Sockets["private_ip"].Required)

	// Auth
	require.NotNil(t, m.Auth)
	assert.Equal(t, "sigv4", m.Auth.Method)
}

func TestHTTPModelParsing(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	m, err := discoverer.GetModel("examples.#HTTPEndpointModel")
	require.NoError(t, err)

	assert.Equal(t, "http_endpoint", m.Metadata.Name)
	assert.Equal(t, "network", m.Metadata.Category)
	assert.Len(t, m.Methods, 3)
	assert.Equal(t, "qualification", m.Methods["health_check"].Kind)
	assert.Contains(t, m.Methods["health_check"].Blocks, "get")
	assert.Contains(t, m.Methods["health_check"].Blocks, "post")

	require.NotNil(t, m.Auth)
	assert.Equal(t, "bearer", m.Auth.Method)
}

func TestSimpleModelParsing(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	m, err := discoverer.GetModel("examples.#SimpleModel")
	require.NoError(t, err)

	assert.Equal(t, "simple", m.Metadata.Name)
	assert.Equal(t, "custom", m.Metadata.Category)
	assert.Len(t, m.Methods, 1)
	assert.Equal(t, "action", m.Methods["get"].Kind)
	assert.Len(t, m.Sockets, 0)
	assert.Nil(t, m.Auth)
}

func TestLifecycleResolution(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	m, err := discoverer.GetModel("examples.#EC2InstanceModel")
	require.NoError(t, err)

	t.Run("create has qualifications", func(t *testing.T) {
		lc, err := ResolveLifecycle(m, "create")
		require.NoError(t, err)
		assert.Equal(t, "create", lc.Action)
		// Both valid_credentials and ami_exists should block create
		assert.Len(t, lc.Qualifications, 2)
		assert.Contains(t, lc.Qualifications, "valid_credentials")
		assert.Contains(t, lc.Qualifications, "ami_exists")
	})

	t.Run("list has qualifications", func(t *testing.T) {
		lc, err := ResolveLifecycle(m, "list")
		require.NoError(t, err)
		assert.Equal(t, "list", lc.Action)
		assert.Len(t, lc.Qualifications, 1)
		assert.Contains(t, lc.Qualifications, "valid_credentials")
	})

	t.Run("nonexistent method", func(t *testing.T) {
		_, err := ResolveLifecycle(m, "nonexistent")
		assert.Error(t, err)
	})
}

func TestLifecycleMinimalModel(t *testing.T) {
	schemaDir := setupTestSchemaDir(t)
	discoverer := NewDiscoverer(schemaDir)

	m, err := discoverer.GetModel("examples.#SimpleModel")
	require.NoError(t, err)

	lc, err := ResolveLifecycle(m, "get")
	require.NoError(t, err)
	assert.Equal(t, "get", lc.Action)
	assert.Empty(t, lc.Qualifications)
}
