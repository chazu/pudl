package validator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reviewSchema(review string) string {
	return `package reviewed

#Bucket: {
	_pudl: {
		schema_type:   "base"
		resource_type: "test.bucket"
		` + review + `
	}
	name: string
}
`
}

func TestReviewMetadata_Decode(t *testing.T) {
	tests := []struct {
		name    string
		review  string
		want    SchemaReview
		wantErr string
	}{
		{name: "absent defaults to draft", review: "", want: SchemaReview{Status: ReviewDraft}},
		{name: "draft", review: `review: status: "draft"`, want: SchemaReview{Status: ReviewDraft}},
		{
			name:   "validated with by and at",
			review: `review: {status: "validated", by: "sre-platform", at: "2026-11-01"}`,
			want:   SchemaReview{Status: ReviewValidated, By: "sre-platform", At: "2026-11-01"},
		},
		{name: "invalid status", review: `review: status: "approved"`, wantErr: `invalid status "approved"`},
		{name: "missing status", review: `review: by: "me"`, wantErr: "status is required"},
		{name: "unknown key", review: `review: {status: "draft", when: "now"}`, wantErr: `unknown field "when"`},
		{name: "non-string by", review: `review: {status: "draft", by: 3}`, wantErr: "by must be a string"},
		{name: "not concrete", review: `review: status: string`, wantErr: "review:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetSharedLoaders()
			t.Cleanup(ResetSharedLoaders)
			dir := schemaDir(t)
			writeSchema(t, dir, filepath.Join("reviewed", "bucket.cue"), reviewSchema(tt.review))

			set := LoadSchemaSet([]string{dir})
			meta, ok := set.Metadata["reviewed.#Bucket"]
			require.True(t, ok)
			if tt.wantErr == "" {
				assert.Empty(t, set.Errors)
				assert.Equal(t, tt.want, meta.Review)
				assert.Equal(t, tt.want.Status == ReviewValidated, meta.IsValidated())
				return
			}
			require.Len(t, set.Errors, 1, "an invalid review is a schema load error")
			assert.Contains(t, set.Errors[0].Error(), "reviewed.#Bucket has invalid _pudl.review")
			assert.Contains(t, set.Errors[0].Error(), tt.wantErr)
			assert.Equal(t, ReviewDraft, meta.Review.Status, "an invalid review never counts as validated")
		})
	}
}

func TestSchemaReview_Describe(t *testing.T) {
	assert.Equal(t, "draft", SchemaReview{}.Describe())
	assert.Equal(t, "validated by sre at 2026-11-01",
		SchemaReview{Status: ReviewValidated, By: "sre", At: "2026-11-01"}.Describe())
}
