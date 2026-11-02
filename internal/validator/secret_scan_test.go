package validator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretScanIgnore_Decode(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		want    []string
		wantErr string
	}{
		{name: "absent", field: "", want: nil},
		{name: "paths", field: `secret_scan_ignore: ["spec.nonce", "items[*].\"x.y\""]`, want: []string{"spec.nonce", `items[*]."x.y"`}},
		{name: "not a list", field: `secret_scan_ignore: "spec.nonce"`, wantErr: "must be a list of field paths"},
		{name: "not strings", field: `secret_scan_ignore: [1]`, wantErr: "must be a list of field paths"},
		{name: "bad path", field: `secret_scan_ignore: ["a..b"]`, wantErr: "empty segment"},
		{name: "not concrete", field: `secret_scan_ignore: [string]`, wantErr: "secret_scan_ignore:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetSharedLoaders()
			t.Cleanup(ResetSharedLoaders)
			dir := schemaDir(t)
			writeSchema(t, dir, filepath.Join("reviewed", "bucket.cue"), reviewSchema(tt.field))

			set := LoadSchemaSet([]string{dir})
			meta, ok := set.Metadata["reviewed.#Bucket"]
			require.True(t, ok)
			if tt.wantErr == "" {
				assert.Empty(t, set.Errors)
				assert.Equal(t, tt.want, meta.SecretScanIgnore)
				return
			}
			require.Len(t, set.Errors, 1, "an invalid allowlist is a schema load error")
			assert.Contains(t, set.Errors[0].Error(), "reviewed.#Bucket has invalid _pudl.secret_scan_ignore")
			assert.Contains(t, set.Errors[0].Error(), tt.wantErr)
			assert.Empty(t, meta.SecretScanIgnore, "an invalid allowlist suppresses nothing")
		})
	}
}
