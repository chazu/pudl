package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseWorkflowsFromFile(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, "workflows")
	os.MkdirAll(wfDir, 0755)

	cueContent := `package workflows

file_pipeline: {
    description: "Create and verify files on remote host"
    abort_on_failure: true

    steps: {
        create_file: {
            definition: "test_file_writer"
            method:     "create_file"
            timeout:    "30s"
        }
        read_file: {
            definition: "test_file_reader"
            method:     "read_file"
            inputs: {
                file_path: steps.create_file.outputs.file_path
            }
        }
        verify: {
            definition: "test_file_verifier"
            method:     "verify_file"
            inputs: {
                content: steps.read_file.outputs.content
            }
        }
    }
}
`
	os.WriteFile(filepath.Join(wfDir, "pipeline.cue"), []byte(cueContent), 0644)

	disc := NewDiscoverer(dir)
	workflows, err := disc.ListWorkflows()
	if err != nil {
		t.Fatalf("ListWorkflows failed: %v", err)
	}

	if len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(workflows))
	}

	wf := workflows[0]
	if wf.Name != "file_pipeline" {
		t.Errorf("expected name 'file_pipeline', got %q", wf.Name)
	}
	if wf.Description != "Create and verify files on remote host" {
		t.Errorf("unexpected description: %q", wf.Description)
	}
	if !wf.AbortOnFailure {
		t.Error("expected abort_on_failure to be true")
	}
	if len(wf.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(wf.Steps))
	}

	// Check create_file step
	step := wf.Steps["create_file"]
	if step.Definition != "test_file_writer" {
		t.Errorf("expected definition 'test_file_writer', got %q", step.Definition)
	}
	if step.Method != "create_file" {
		t.Errorf("expected method 'create_file', got %q", step.Method)
	}
	if step.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", step.Timeout)
	}

	// Check read_file step has inputs
	step = wf.Steps["read_file"]
	if step.Definition != "test_file_reader" {
		t.Errorf("expected definition 'test_file_reader', got %q", step.Definition)
	}
	if len(step.Inputs) == 0 {
		t.Error("expected read_file to have inputs")
	}
	if ref, ok := step.Inputs["file_path"]; !ok || ref != "steps.create_file.outputs.file_path" {
		t.Errorf("expected file_path input ref, got %q", ref)
	}

	// Check verify step
	step = wf.Steps["verify"]
	if ref, ok := step.Inputs["content"]; !ok || ref != "steps.read_file.outputs.content" {
		t.Errorf("expected content input ref, got %q", ref)
	}
}

func TestGetWorkflow(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, "workflows")
	os.MkdirAll(wfDir, 0755)

	cueContent := `package workflows

simple: {
    description: "A simple workflow"
    steps: {
        step1: {
            definition: "def1"
            method:     "action1"
        }
    }
}
`
	os.WriteFile(filepath.Join(wfDir, "simple.cue"), []byte(cueContent), 0644)

	disc := NewDiscoverer(dir)

	wf, err := disc.GetWorkflow("simple")
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if wf.Name != "simple" {
		t.Errorf("expected name 'simple', got %q", wf.Name)
	}

	_, err = disc.GetWorkflow("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent workflow")
	}
}

func TestParseAbortOnFailureFalse(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, "workflows")
	os.MkdirAll(wfDir, 0755)

	cueContent := `package workflows

lenient: {
    description: "Non-aborting workflow"
    abort_on_failure: false

    steps: {
        step1: {
            definition: "def1"
            method:     "action1"
        }
    }
}
`
	os.WriteFile(filepath.Join(wfDir, "lenient.cue"), []byte(cueContent), 0644)

	disc := NewDiscoverer(dir)
	wf, err := disc.GetWorkflow("lenient")
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if wf.AbortOnFailure {
		t.Error("expected abort_on_failure to be false")
	}
}

func TestParseStepRetries(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, "workflows")
	os.MkdirAll(wfDir, 0755)

	cueContent := `package workflows

retry_wf: {
    description: "Workflow with retries"
    steps: {
        flaky: {
            definition: "def1"
            method:     "action1"
            timeout:    "1m"
            retries:    3
        }
    }
}
`
	os.WriteFile(filepath.Join(wfDir, "retry.cue"), []byte(cueContent), 0644)

	disc := NewDiscoverer(dir)
	wf, err := disc.GetWorkflow("retry_wf")
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}

	step := wf.Steps["flaky"]
	if step.Retries != 3 {
		t.Errorf("expected 3 retries, got %d", step.Retries)
	}
	if step.Timeout != time.Minute {
		t.Errorf("expected 1m timeout, got %v", step.Timeout)
	}
}

func TestNoWorkflowsDir(t *testing.T) {
	dir := t.TempDir()
	disc := NewDiscoverer(dir)
	workflows, err := disc.ListWorkflows()
	if err != nil {
		t.Fatalf("ListWorkflows failed: %v", err)
	}
	if len(workflows) != 0 {
		t.Errorf("expected 0 workflows, got %d", len(workflows))
	}
}
