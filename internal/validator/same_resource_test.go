package validator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/sameresource"
)

func sameResourceSchema(decl string) string {
	return `package linked

#Cluster: {
	_pudl: {
		schema_type:   "base"
		resource_type: "test.cluster"
		identity_fields: ["name"]
		` + decl + `
	}
	name: string
	...
}
`
}

func TestSameResourceMetadata_Decode(t *testing.T) {
	tests := []struct {
		name, decl, wantErr string
	}{
		{name: "absent", decl: ""},
		{name: "two keys for one kind", decl: `same_resource: [{kind: "k8s.cluster", key: host: "endpoint"}, {kind: "k8s.cluster", key: host: {path: "server", trim_prefix: "https://"}}]`},
		{name: "unknown key", decl: `same_resource: [{kind: "k8s.cluster", key: host: "endpoint", match: true}]`, wantErr: `unknown field "match"`},
		{name: "not concrete", decl: `same_resource: [{kind: string, key: host: "endpoint"}]`, wantErr: "same_resource:"},
		{name: "bad kind", decl: `same_resource: [{kind: "Cluster", key: host: "endpoint"}]`, wantErr: "dotted lower-case words"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetSharedLoaders()
			t.Cleanup(ResetSharedLoaders)
			dir := schemaDir(t)
			writeSchema(t, dir, filepath.Join("linked", "cluster.cue"), sameResourceSchema(tt.decl))

			set := LoadSchemaSet([]string{dir})
			meta, ok := set.Metadata["linked.#Cluster"]
			require.True(t, ok)
			if tt.wantErr == "" {
				assert.Empty(t, set.Errors)
				assert.Empty(t, meta.SameResourceError)
				if tt.decl != "" {
					require.NotNil(t, meta.SameResource)
					keys := meta.SameResource.Keys(map[string]any{"endpoint": "10.0.0.2", "server": "https://34.1.2.3"})
					assert.Equal(t, []sameresource.Key{
						{Kind: "k8s.cluster", Key: `{"host":"10.0.0.2"}`},
						{Kind: "k8s.cluster", Key: `{"host":"34.1.2.3"}`},
					}, keys)
				}
				return
			}
			require.Len(t, set.Errors, 1, "invalid same_resource metadata is a schema load error")
			assert.Contains(t, set.Errors[0].Error(), "linked.#Cluster has invalid _pudl.same_resource")
			assert.Contains(t, set.Errors[0].Error(), tt.wantErr)
			assert.Nil(t, meta.SameResource)
		})
	}
}
