package validator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ownerSchema(owner string) string {
	return `package owned

_projectOwners: {"proj-a": "payments"}

#Bucket: {
	_pudl: {
		schema_type:   "base"
		resource_type: "test.bucket"
		identity_fields: ["name"]
		` + owner + `
	}
	name: string
	...
}
`
}

func TestOwnerMetadata_Decode(t *testing.T) {
	tests := []struct {
		name, owner, wantErr string
	}{
		{name: "absent", owner: ""},
		{name: "paths, shared map and fallback", owner: `owner: {paths: ["labels.team"], map: {path: "project", owners: _projectOwners}, fallback: "platform"}`},
		{name: "unknown key", owner: `owner: {paths: ["labels.team"], default: "x"}`, wantErr: `unknown field "default"`},
		{name: "not concrete", owner: `owner: paths: [string]`, wantErr: "owner:"},
		{name: "wildcard path", owner: `owner: paths: ["tags[*]"]`, wantErr: "cannot contain [*]"},
		{name: "nothing to derive from", owner: `owner: {}`, wantErr: "declare at least one of paths, map or fallback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetSharedLoaders()
			t.Cleanup(ResetSharedLoaders)
			dir := schemaDir(t)
			writeSchema(t, dir, filepath.Join("owned", "bucket.cue"), ownerSchema(tt.owner))

			set := LoadSchemaSet([]string{dir})
			meta, ok := set.Metadata["owned.#Bucket"]
			require.True(t, ok)
			if tt.wantErr == "" {
				assert.Empty(t, set.Errors)
				assert.Empty(t, meta.OwnerError)
				if tt.owner != "" {
					require.NotNil(t, meta.Owner)
					owner, via, ok := meta.Owner.Resolve(map[string]any{"project": "proj-a"})
					assert.True(t, ok)
					assert.Equal(t, "payments", owner, "the package-level table is shared by reference")
					assert.Equal(t, "map:project", via)
				}
				return
			}
			require.Len(t, set.Errors, 1, "invalid owner metadata is a schema load error")
			assert.Contains(t, set.Errors[0].Error(), "owned.#Bucket has invalid _pudl.owner")
			assert.Contains(t, set.Errors[0].Error(), tt.wantErr)
			assert.Nil(t, meta.Owner)
		})
	}
}
