package workflow

import (
	"testing"
	"time"
)

func TestManifestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewManifestStore(dir)

	now := time.Now()
	m := &Manifest{
		WorkflowName: "test_wf",
		RunID:        "20260307T120000",
		Status:       "success",
		StartTime:    now,
		EndTime:      now.Add(5 * time.Second),
		DurationSecs: 5.0,
		Steps: map[string]StepManifest{
			"step1": {
				StepName:       "step1",
				DefinitionName: "def1",
				MethodName:     "action1",
				Status:         "success",
				StartTime:      now.Format(time.RFC3339),
				EndTime:        now.Add(2 * time.Second).Format(time.RFC3339),
				DurationSecs:   2.0,
				Attempt:        1,
			},
		},
	}

	if err := store.Save(m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Get("test_wf", "20260307T120000")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if loaded.WorkflowName != "test_wf" {
		t.Errorf("expected workflow name 'test_wf', got %q", loaded.WorkflowName)
	}
	if loaded.Status != "success" {
		t.Errorf("expected status 'success', got %q", loaded.Status)
	}
	if len(loaded.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(loaded.Steps))
	}
}

func TestManifestList(t *testing.T) {
	dir := t.TempDir()
	store := NewManifestStore(dir)

	now := time.Now()
	for _, id := range []string{"20260307T100000", "20260307T110000", "20260307T120000"} {
		m := &Manifest{
			WorkflowName: "test_wf",
			RunID:        id,
			Status:       "success",
			StartTime:    now,
			EndTime:      now,
			Steps:        map[string]StepManifest{},
		}
		if err := store.Save(m); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	ids, err := store.List("test_wf")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(ids) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(ids))
	}

	// Should be sorted newest first
	if ids[0] != "20260307T120000" {
		t.Errorf("expected newest first, got %q", ids[0])
	}
}

func TestManifestGetNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewManifestStore(dir)

	_, err := store.Get("nonexistent", "run1")
	if err == nil {
		t.Error("expected error for nonexistent manifest")
	}
}

func TestManifestListEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewManifestStore(dir)

	ids, err := store.List("nonexistent")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 runs, got %d", len(ids))
	}
}
