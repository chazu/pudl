package mubridge

import (
	"testing"
	"time"

	"pudl/internal/drift"
)

func TestExportFromDriftReport_NilReport(t *testing.T) {
	resp := ExportFromDriftReport(nil)
	if resp.Error == "" {
		t.Fatal("expected error for nil report")
	}
}

func TestExportFromDriftReport_NoDifferences(t *testing.T) {
	report := &drift.DriftResult{
		Definition:  "test_def",
		Status:      "clean",
		Timestamp:   time.Now(),
		Differences: nil,
	}
	resp := ExportFromDriftReport(report)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Actions) != 0 {
		t.Fatalf("expected 0 actions, got %d", len(resp.Actions))
	}
	if resp.Outputs["status"] != "clean" {
		t.Fatalf("expected status 'clean', got %q", resp.Outputs["status"])
	}
}

func TestExportFromDriftReport_WithDifferences(t *testing.T) {
	report := &drift.DriftResult{
		Definition: "my_instance",
		Status:     "drifted",
		Timestamp:  time.Now(),
		Differences: []drift.FieldDiff{
			{Path: "spec.replicas", Type: "changed", Declared: 3, Live: 5},
			{Path: "metadata.extra_label", Type: "added", Declared: nil, Live: "new-value"},
			{Path: "spec.old_field", Type: "removed", Declared: "gone", Live: nil},
		},
	}

	resp := ExportFromDriftReport(report)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(resp.Actions))
	}

	// Verify changed action
	changed := resp.Actions[0]
	if changed.ID != "my_instance-drift-0" {
		t.Errorf("unexpected action ID: %s", changed.ID)
	}
	if changed.Inputs["type"] != "changed" {
		t.Errorf("expected type 'changed', got %q", changed.Inputs["type"])
	}
	if changed.Inputs["field"] != "spec.replicas" {
		t.Errorf("expected field 'spec.replicas', got %q", changed.Inputs["field"])
	}
	if len(changed.Command) != 2 || changed.Command[0] != "echo" {
		t.Errorf("expected echo command, got %v", changed.Command)
	}

	// Verify added action
	added := resp.Actions[1]
	if added.Inputs["type"] != "added" {
		t.Errorf("expected type 'added', got %q", added.Inputs["type"])
	}

	// Verify removed action
	removed := resp.Actions[2]
	if removed.Inputs["type"] != "removed" {
		t.Errorf("expected type 'removed', got %q", removed.Inputs["type"])
	}

	// Verify outputs
	if resp.Outputs["definition"] != "my_instance" {
		t.Errorf("expected definition 'my_instance', got %q", resp.Outputs["definition"])
	}
	if resp.Outputs["status"] != "drifted" {
		t.Errorf("expected status 'drifted', got %q", resp.Outputs["status"])
	}
}
