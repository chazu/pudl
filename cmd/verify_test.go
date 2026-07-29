package cmd

import (
	"testing"

	"github.com/chazu/pudl/internal/database"
)

func TestShouldVerifyInferenceOnlyForUnassignedImports(t *testing.T) {
	entryType := "observe"
	collectionType := "collection"

	tests := []struct {
		name  string
		data  interface{}
		entry database.CatalogEntry
		want  bool
	}{
		{
			name: "ordinary import",
			data: map[string]interface{}{"name": "fixture"},
			want: true,
		},
		{
			name: "declared schema",
			data: map[string]interface{}{"_schema": "k8s.resource", "kind": "Pod"},
			want: false,
		},
		{
			name:  "bridge entry",
			data:  map[string]interface{}{"name": "manifest"},
			entry: database.CatalogEntry{EntryType: &entryType},
			want:  false,
		},
		{
			name:  "collection entry",
			data:  map[string]interface{}{"name": "snapshot"},
			entry: database.CatalogEntry{CollectionType: &collectionType},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldVerifyInference(tt.data, tt.entry); got != tt.want {
				t.Fatalf("shouldVerifyInference() = %v, want %v", got, tt.want)
			}
		})
	}
}
