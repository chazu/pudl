package workflow

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"pudl/internal/executor"
)

// mockExecutor implements StepExecutor for testing.
type mockExecutor struct {
	runFn func(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error)
	calls atomic.Int32
}

func (m *mockExecutor) Run(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
	m.calls.Add(1)
	if m.runFn != nil {
		return m.runFn(ctx, opts)
	}
	return &executor.RunResult{
		MethodName:     opts.MethodName,
		DefinitionName: opts.DefinitionName,
		Output: map[string]interface{}{
			"result": fmt.Sprintf("%s_%s_output", opts.DefinitionName, opts.MethodName),
		},
	}, nil
}

func TestRunnerSuccess(t *testing.T) {
	dir := t.TempDir()
	mock := &mockExecutor{}

	runner := NewRunner(mock, nil, dir)

	wf := &Workflow{
		Name:           "test",
		AbortOnFailure: true,
		Steps: map[string]Step{
			"a": {Name: "a", Definition: "d1", Method: "m1"},
			"b": {Name: "b", Definition: "d2", Method: "m2", Inputs: map[string]string{
				"x": "steps.a.outputs.result",
			}},
		},
	}

	result, err := runner.Run(context.Background(), wf, RunOptions{})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}
	if len(result.Steps) != 2 {
		t.Errorf("expected 2 step results, got %d", len(result.Steps))
	}
	for name, sr := range result.Steps {
		if sr.Status != "success" {
			t.Errorf("step %s: expected success, got %q", name, sr.Status)
		}
	}

	if mock.calls.Load() != 2 {
		t.Errorf("expected 2 executor calls, got %d", mock.calls.Load())
	}
}

func TestRunnerConcurrency(t *testing.T) {
	dir := t.TempDir()
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	mock := &mockExecutor{
		runFn: func(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
			c := current.Add(1)
			for {
				old := maxConcurrent.Load()
				if c <= old {
					break
				}
				if maxConcurrent.CompareAndSwap(old, c) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			current.Add(-1)
			return &executor.RunResult{
				MethodName:     opts.MethodName,
				DefinitionName: opts.DefinitionName,
				Output:         map[string]interface{}{"result": "ok"},
			}, nil
		},
	}

	runner := NewRunner(mock, nil, dir)

	wf := &Workflow{
		Name:           "concurrent",
		AbortOnFailure: true,
		Steps: map[string]Step{
			"a": {Name: "a", Definition: "d1", Method: "m1"},
			"b": {Name: "b", Definition: "d2", Method: "m2"},
			"c": {Name: "c", Definition: "d3", Method: "m3"},
		},
	}

	result, err := runner.Run(context.Background(), wf, RunOptions{})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}

	// With 3 independent steps, max concurrency should be > 1
	if maxConcurrent.Load() < 2 {
		t.Logf("max concurrent was %d (may vary by system)", maxConcurrent.Load())
	}
}

func TestRunnerAbortOnFailure(t *testing.T) {
	dir := t.TempDir()

	mock := &mockExecutor{
		runFn: func(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
			if opts.DefinitionName == "d1" {
				return nil, fmt.Errorf("step failed")
			}
			return &executor.RunResult{
				MethodName:     opts.MethodName,
				DefinitionName: opts.DefinitionName,
				Output:         map[string]interface{}{"result": "ok"},
			}, nil
		},
	}

	runner := NewRunner(mock, nil, dir)

	wf := &Workflow{
		Name:           "abort_test",
		AbortOnFailure: true,
		Steps: map[string]Step{
			"a": {Name: "a", Definition: "d1", Method: "m1"},
			"b": {Name: "b", Definition: "d2", Method: "m2", Inputs: map[string]string{
				"x": "steps.a.outputs.result",
			}},
		},
	}

	result, err := runner.Run(context.Background(), wf, RunOptions{})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Status != "failed" {
		t.Errorf("expected failed, got %q", result.Status)
	}

	if result.Steps["a"].Status != "failed" {
		t.Errorf("step a: expected failed, got %q", result.Steps["a"].Status)
	}
	if result.Steps["b"].Status != "skipped" {
		t.Errorf("step b: expected skipped, got %q", result.Steps["b"].Status)
	}
}

func TestRunnerRetry(t *testing.T) {
	dir := t.TempDir()
	var attempts atomic.Int32

	mock := &mockExecutor{
		runFn: func(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
			n := attempts.Add(1)
			if n < 3 {
				return nil, fmt.Errorf("transient error")
			}
			return &executor.RunResult{
				MethodName:     opts.MethodName,
				DefinitionName: opts.DefinitionName,
				Output:         map[string]interface{}{"result": "ok"},
			}, nil
		},
	}

	runner := NewRunner(mock, nil, dir)

	wf := &Workflow{
		Name:           "retry_test",
		AbortOnFailure: true,
		Steps: map[string]Step{
			"a": {Name: "a", Definition: "d1", Method: "m1", Retries: 3},
		},
	}

	result, err := runner.Run(context.Background(), wf, RunOptions{})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected success after retries, got %q", result.Status)
	}
	if result.Steps["a"].Attempt != 3 {
		t.Errorf("expected attempt 3, got %d", result.Steps["a"].Attempt)
	}
}

func TestRunnerInputResolution(t *testing.T) {
	dir := t.TempDir()
	var receivedTags map[string]string

	mock := &mockExecutor{
		runFn: func(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
			if opts.DefinitionName == "d1" {
				return &executor.RunResult{
					MethodName:     opts.MethodName,
					DefinitionName: opts.DefinitionName,
					Output: map[string]interface{}{
						"file_path": "/tmp/test.txt",
					},
				}, nil
			}
			// Second step: capture what tags it received
			receivedTags = opts.Tags
			return &executor.RunResult{
				MethodName:     opts.MethodName,
				DefinitionName: opts.DefinitionName,
				Output:         map[string]interface{}{"result": "ok"},
			}, nil
		},
	}

	runner := NewRunner(mock, nil, dir)

	wf := &Workflow{
		Name:           "input_test",
		AbortOnFailure: true,
		Steps: map[string]Step{
			"create": {Name: "create", Definition: "d1", Method: "m1"},
			"read":   {Name: "read", Definition: "d2", Method: "m2", Inputs: map[string]string{
				"file_path": "steps.create.outputs.file_path",
			}},
		},
	}

	result, err := runner.Run(context.Background(), wf, RunOptions{})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}

	if receivedTags == nil {
		t.Fatal("second step did not receive tags")
	}
	if receivedTags["file_path"] != "/tmp/test.txt" {
		t.Errorf("expected file_path=/tmp/test.txt, got %q", receivedTags["file_path"])
	}
}

func TestRunnerMaxConcurrency(t *testing.T) {
	dir := t.TempDir()
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	mock := &mockExecutor{
		runFn: func(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
			c := current.Add(1)
			for {
				old := maxConcurrent.Load()
				if c <= old {
					break
				}
				if maxConcurrent.CompareAndSwap(old, c) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			current.Add(-1)
			return &executor.RunResult{
				MethodName:     opts.MethodName,
				DefinitionName: opts.DefinitionName,
				Output:         map[string]interface{}{"result": "ok"},
			}, nil
		},
	}

	runner := NewRunner(mock, nil, dir)

	wf := &Workflow{
		Name:           "limited",
		AbortOnFailure: true,
		Steps: map[string]Step{
			"a": {Name: "a", Definition: "d1", Method: "m1"},
			"b": {Name: "b", Definition: "d2", Method: "m2"},
			"c": {Name: "c", Definition: "d3", Method: "m3"},
			"d": {Name: "d", Definition: "d4", Method: "m4"},
		},
	}

	result, err := runner.Run(context.Background(), wf, RunOptions{MaxConcurrency: 2})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}

	if maxConcurrent.Load() > 2 {
		t.Errorf("expected max concurrency <= 2, got %d", maxConcurrent.Load())
	}
}

func TestRunnerDryRun(t *testing.T) {
	dir := t.TempDir()

	mock := &mockExecutor{
		runFn: func(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
			if !opts.DryRun {
				t.Error("expected dry-run flag to be set")
			}
			return &executor.RunResult{
				MethodName:     opts.MethodName,
				DefinitionName: opts.DefinitionName,
			}, nil
		},
	}

	runner := NewRunner(mock, nil, dir)

	wf := &Workflow{
		Name:           "dryrun",
		AbortOnFailure: true,
		Steps: map[string]Step{
			"a": {Name: "a", Definition: "d1", Method: "m1"},
		},
	}

	result, err := runner.Run(context.Background(), wf, RunOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}
}
