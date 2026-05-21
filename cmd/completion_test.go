package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCompleteFormats(t *testing.T) {
	tests := []struct {
		name       string
		toComplete string
		wantLen    int
		wantFirst  string
	}{
		{"empty prefix returns all", "", 4, "json"},
		{"j prefix", "j", 1, "json"},
		{"n prefix", "n", 1, "ndjson"},
		{"no match", "x", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, directive := completeFormats(nil, nil, tt.toComplete)
			assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
			assert.Len(t, results, tt.wantLen)
			if tt.wantLen > 0 {
				assert.Equal(t, tt.wantFirst, results[0])
			}
		})
	}
}

func TestCompleteSortByOptions(t *testing.T) {
	results, directive := completeSortByOptions(nil, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Len(t, results, 5)

	// Filter by prefix
	results, _ = completeSortByOptions(nil, nil, "s")
	assert.Len(t, results, 2) // size, schema
}

func TestCompleteObservationKinds(t *testing.T) {
	results, directive := completeObservationKinds(nil, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Len(t, results, 7)

	// Filter by prefix
	results, _ = completeObservationKinds(nil, nil, "b")
	assert.Len(t, results, 1)
	assert.Contains(t, results[0], "bug")

	// No match
	results, _ = completeObservationKinds(nil, nil, "z")
	assert.Empty(t, results)
}

func TestCompleteFormatsAllValues(t *testing.T) {
	results, _ := completeFormats(nil, nil, "")
	expected := []string{"json", "yaml", "csv", "ndjson"}
	assert.Equal(t, expected, results)
}

func TestCompleteObservationKindsFiltering(t *testing.T) {
	tests := []struct {
		prefix    string
		wantCount int
	}{
		{"", 7},
		{"b", 1},
		{"o", 2}, // obstacle, opportunity
		{"d", 2}, // decision, debt
		{"r", 1}, // risk
		{"p", 1}, // pattern
		{"x", 0},
	}

	for _, tt := range tests {
		t.Run("prefix_"+tt.prefix, func(t *testing.T) {
			results, _ := completeObservationKinds(nil, nil, tt.prefix)
			assert.Len(t, results, tt.wantCount)
		})
	}
}
