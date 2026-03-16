package drift

import (
	"testing"
	"time"
)

func TestReportStore_SaveAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewReportStore(tmpDir)

	result := &DriftResult{
		Definition: "my_instance",

		Status:     "drifted",
		Timestamp:  time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC),
		DeclaredKeys: map[string]interface{}{
			"vpc_id": "vpc-123",
		},
		LiveState: map[string]interface{}{
			"vpc_id": "vpc-456",
		},
		Differences: []FieldDiff{
			{
				Path:     "vpc_id",
				Type:     "changed",
				Declared: "vpc-123",
				Live:     "vpc-456",
			},
		},
	}

	if err := store.Save(result); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// List
	ids, err := store.List("my_instance")
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 report, got %d", len(ids))
	}

	// Get
	loaded, err := store.Get("my_instance", ids[0])
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if loaded.Definition != "my_instance" {
		t.Errorf("expected definition 'my_instance', got %q", loaded.Definition)
	}
	if loaded.Status != "drifted" {
		t.Errorf("expected status 'drifted', got %q", loaded.Status)
	}
	if len(loaded.Differences) != 1 {
		t.Errorf("expected 1 diff, got %d", len(loaded.Differences))
	}
}

func TestReportStore_GetLatest(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewReportStore(tmpDir)

	// Save two reports with different timestamps
	r1 := &DriftResult{
		Definition: "test",

		Status:     "clean",
		Timestamp:  time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC),
	}
	r2 := &DriftResult{
		Definition: "test",

		Status:     "drifted",
		Timestamp:  time.Date(2026, 3, 7, 14, 0, 0, 0, time.UTC),
	}

	store.Save(r1)
	store.Save(r2)

	latest, err := store.GetLatest("test")
	if err != nil {
		t.Fatalf("failed to get latest: %v", err)
	}

	if latest.Status != "drifted" {
		t.Errorf("expected latest to be 'drifted', got %q", latest.Status)
	}
}

func TestReportStore_ListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewReportStore(tmpDir)

	ids, err := store.List("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty list, got %d", len(ids))
	}
}

func TestReportStore_GetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewReportStore(tmpDir)

	_, err := store.Get("test", "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent report")
	}
}

func TestReportStore_GetLatestEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewReportStore(tmpDir)

	_, err := store.GetLatest("nonexistent")
	if err == nil {
		t.Error("expected error for empty report list")
	}
}
