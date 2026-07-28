package mubridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/muschemas"
	"github.com/chazu/pudl/internal/validator"
)

func TestLoadPluginPackageAndSyncSchemas(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mu.cue"), []byte(`package mu

plugin: {
	entrypoint: "plugin.bb"
	schemas: [{module: "mu/aws", version: "v1", path: "schemas/mu/aws"}]
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pudl.cue"), []byte(`package pudl

mappings: [{resource_type: "aws.ec2.vpc", schema: "pudl/aws.#VPC"}]
`), 0o644))
	schemaDir := filepath.Join(dir, "schemas", "mu", "aws")
	require.NoError(t, os.MkdirAll(schemaDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "aws.cue"), []byte("package aws\n#VPC: {}\n"), 0o644))

	cache, err := muschemas.New(filepath.Join(t.TempDir(), "schemas"))
	require.NoError(t, err)
	pkg, err := SyncPluginSchemas(cache, dir)
	require.NoError(t, err)
	require.Len(t, pkg.Schemas, 1)
	require.Len(t, pkg.Mappings, 1)
	assert.True(t, cache.Has("mu/aws", "v1"))
	files, err := cache.Files("mu/aws", "v1")
	require.NoError(t, err)
	assert.Equal(t, []string{"aws.cue"}, files)
}

func TestSyncPluginSchemasRejectsPudlNamespace(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mu.cue"), []byte(`package mu
plugin: {entrypoint: "plugin.bb", schemas: [{module: "pudl/aws", version: "v1", path: "schemas"}]}
`), 0o644))
	cache, err := muschemas.New(filepath.Join(t.TempDir(), "schemas"))
	require.NoError(t, err)
	_, err = SyncPluginSchemas(cache, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved PUDL namespace")
}

func TestValidatePluginMappings(t *testing.T) {
	graph := inference.BuildInheritanceGraph(map[string]validator.SchemaMetadata{
		"pudl/aws.#VPC": {ResourceType: "aws.ec2.vpc"},
	})
	tests := []struct {
		name string
		maps []PluginMapping
		want string
	}{
		{name: "valid", maps: []PluginMapping{{ResourceType: "aws.ec2.vpc", Schema: "pudl/aws.#VPC"}}},
		{name: "duplicate", maps: []PluginMapping{
			{ResourceType: "aws.ec2.vpc", Schema: "pudl/aws.#VPC"},
			{ResourceType: "aws.ec2.vpc", Schema: "pudl/aws.#VPC"},
		}, want: "duplicate resource_type"},
		{name: "foreign namespace", maps: []PluginMapping{{ResourceType: "aws.ec2.vpc", Schema: "mu/aws.#VPC"}}, want: "PUDL namespace"},
		{name: "missing schema", maps: []PluginMapping{{ResourceType: "aws.ec2.vpc", Schema: "pudl/aws.#Missing"}}, want: "not loaded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePluginMappings(tt.maps, graph)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
	assert.NoError(t, ValidatePluginMappings(nil, graph))
	assert.Error(t, ValidatePluginMappings([]PluginMapping{{ResourceType: "x", Schema: "pudl/x.#X"}}, nil))
}

func TestSyncInstalledPluginSchemas(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	digest := "sha256:" + strings.Repeat("a", 64)
	bundle := filepath.Join(home, ".mu", "plugins", "aws", "bundle-aaaaaaaaaaaa")
	schemaDir := filepath.Join(bundle, "schemas", "mu", "aws")
	require.NoError(t, os.MkdirAll(schemaDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "aws.cue"), []byte("package aws\n#VPC: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "mu-plugin.json"), []byte(`{
  "name": "aws",
  "version": "0.1.0",
  "bundle_digest": "`+digest+`",
  "schemas": [{"module": "mu/aws", "version": "v1", "path": "schemas/mu/aws"}],
  "pudl_mappings": [{"resource_type": "aws.ec2.vpc", "schema": "pudl/aws.#VPC"}]
}`), 0o644))

	cache, err := muschemas.New(filepath.Join(t.TempDir(), "schemas"))
	require.NoError(t, err)
	pkg, err := SyncInstalledPluginSchemas(cache, t.TempDir(), "aws", digest)
	require.NoError(t, err)
	assert.Equal(t, "pudl/aws.#VPC", pkg.Mappings[0].Schema)
	assert.True(t, cache.Has("mu/aws", "v1"))
}

func TestLoadInstalledPluginPackageMissingSuggestsCatalogInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	digest := "sha256:" + strings.Repeat("b", 64)
	require.NoError(t, os.WriteFile(filepath.Join(project, "mu.lock"), []byte(`{
  "schema_version": 1,
  "plugins": [{"name": "aws", "version": "0.1.0", "bundle_digest": "`+digest+`"}]
}`), 0o644))
	_, _, err := LoadInstalledPluginPackage(project, "aws", digest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mu plugin install aws@0.1.0")
}

func TestLoadInstalledPluginPackageMatchesMuShortBundleDigest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	fullDigest := "sha256:" + strings.Repeat("c", 64)
	bundle := filepath.Join(home, ".mu", "plugins", "aws", "bundle-cccccccccccc")
	require.NoError(t, os.MkdirAll(bundle, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, "mu.lock"), []byte(`{
  "schema_version": 1,
  "plugins": [{"name": "aws", "version": "0.1.0", "bundle_digest": "`+fullDigest+`"}]
}`), 0o644))

	_, gotBundle, err := LoadInstalledPluginPackage(project, "aws", "sha256:cccccccccccc")
	require.NoError(t, err)
	assert.Equal(t, bundle, gotBundle)
}
