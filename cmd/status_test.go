package cmd

import (
	"testing"
	"time"

	"pudl/internal/database"
	"pudl/internal/drift"
)

func TestColorForStatus(t *testing.T) {
	tests := []struct {
		status string
	}{
		{"clean"},
		{"converged"},
		{"drifted"},
		{"converging"},
		{"failed"},
		{"unknown"},
		{""},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			// Should not panic
			style := colorForStatus(tt.status)
			rendered := style.Render(tt.status)
			if rendered == "" && tt.status != "" {
				t.Error("expected non-empty rendered string")
			}
		})
	}
}

func TestFormatStatusTime(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "zero time",
			input:    time.Time{},
			expected: "",
		},
		{
			name:     "valid time",
			input:    time.Date(2026, 3, 24, 10, 15, 0, 0, time.UTC),
			expected: "2026-03-24T10:15:00Z",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStatusTime(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestEnrichDiffCounts_NilStore(t *testing.T) {
	statuses := []database.DefinitionStatus{
		{Definition: "test", Status: "drifted", DiffCount: 0},
	}
	// Should not panic with nil store
	enrichDiffCounts(statuses, nil)
	if statuses[0].DiffCount != 0 {
		t.Errorf("expected DiffCount to remain 0 with nil store, got %d", statuses[0].DiffCount)
	}
}

func TestStatusOutput_JSONFields(t *testing.T) {
	// Verify struct fields exist and are assignable
	out := StatusOutput{
		Definition: "test_def",
		Status:     "clean",
		UpdatedAt:  "2026-03-24T10:15:00Z",
		DiffCount:  0,
	}
	if out.Definition != "test_def" {
		t.Error("unexpected Definition value")
	}
}

func TestStatusDetailOutput_WithDifferences(t *testing.T) {
	out := StatusDetailOutput{
		Definition: "monitoring",
		Status:     "drifted",
		UpdatedAt:  "2026-03-24T10:12:00Z",
		DiffCount:  2,
		Differences: []drift.FieldDiff{
			{Path: "threshold", Type: "changed", Declared: 80, Live: 90},
			{Path: "new_field", Type: "added", Live: "value"},
		},
	}
	if len(out.Differences) != 2 {
		t.Errorf("expected 2 differences, got %d", len(out.Differences))
	}
}
