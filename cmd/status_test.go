package cmd

import (
	"testing"
	"time"
)

func TestColorForStatus(t *testing.T) {
	tests := []struct {
		status string
	}{
		{"clean"},
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

func TestStatusOutput_JSONFields(t *testing.T) {
	// Verify struct fields exist and are assignable
	out := StatusOutput{
		Target: "test_def",
		Status:     "clean",
		UpdatedAt:  "2026-03-24T10:15:00Z",
	}
	if out.Target != "test_def" {
		t.Error("unexpected Target value")
	}
}
