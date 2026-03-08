package workflow

import (
	"strings"
	"testing"
)

func makeWorkflow(steps map[string]Step) *Workflow {
	return &Workflow{
		Name:  "test",
		Steps: steps,
	}
}

func TestLinearChain(t *testing.T) {
	wf := makeWorkflow(map[string]Step{
		"a": {Name: "a", Definition: "d1", Method: "m1"},
		"b": {Name: "b", Definition: "d2", Method: "m2", Inputs: map[string]string{
			"x": "steps.a.outputs.result",
		}},
		"c": {Name: "c", Definition: "d3", Method: "m3", Inputs: map[string]string{
			"y": "steps.b.outputs.result",
		}},
	})

	dag, err := BuildDAG(wf)
	if err != nil {
		t.Fatalf("BuildDAG failed: %v", err)
	}

	order, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(order))
	}

	// a must come before b, b before c
	aIdx, bIdx, cIdx := indexOf(order, "a"), indexOf(order, "b"), indexOf(order, "c")
	if aIdx >= bIdx || bIdx >= cIdx {
		t.Errorf("expected a < b < c, got order: %v", order)
	}
}

func TestDiamondDependency(t *testing.T) {
	wf := makeWorkflow(map[string]Step{
		"root": {Name: "root", Definition: "d1", Method: "m1"},
		"left": {Name: "left", Definition: "d2", Method: "m2", Inputs: map[string]string{
			"x": "steps.root.outputs.result",
		}},
		"right": {Name: "right", Definition: "d3", Method: "m3", Inputs: map[string]string{
			"x": "steps.root.outputs.result",
		}},
		"join": {Name: "join", Definition: "d4", Method: "m4", Inputs: map[string]string{
			"a": "steps.left.outputs.result",
			"b": "steps.right.outputs.result",
		}},
	})

	dag, err := BuildDAG(wf)
	if err != nil {
		t.Fatalf("BuildDAG failed: %v", err)
	}

	order, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(order))
	}

	// root must be first, join must be last
	if order[0] != "root" {
		t.Errorf("expected root first, got %q", order[0])
	}
	if order[3] != "join" {
		t.Errorf("expected join last, got %q", order[3])
	}
}

func TestIndependentSteps(t *testing.T) {
	wf := makeWorkflow(map[string]Step{
		"a": {Name: "a", Definition: "d1", Method: "m1"},
		"b": {Name: "b", Definition: "d2", Method: "m2"},
		"c": {Name: "c", Definition: "d3", Method: "m3"},
	})

	dag, err := BuildDAG(wf)
	if err != nil {
		t.Fatalf("BuildDAG failed: %v", err)
	}

	order, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(order))
	}

	// All should be ready at start
	ready := dag.GetReadySteps(map[string]bool{})
	if len(ready) != 3 {
		t.Errorf("expected 3 ready steps, got %d", len(ready))
	}
}

func TestCycleDetection(t *testing.T) {
	wf := makeWorkflow(map[string]Step{
		"a": {Name: "a", Definition: "d1", Method: "m1", Inputs: map[string]string{
			"x": "steps.b.outputs.result",
		}},
		"b": {Name: "b", Definition: "d2", Method: "m2", Inputs: map[string]string{
			"x": "steps.a.outputs.result",
		}},
	})

	dag, err := BuildDAG(wf)
	if err != nil {
		t.Fatalf("BuildDAG failed: %v", err)
	}

	_, err = dag.TopologicalSort()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestUnknownStepReference(t *testing.T) {
	wf := makeWorkflow(map[string]Step{
		"a": {Name: "a", Definition: "d1", Method: "m1", Inputs: map[string]string{
			"x": "steps.nonexistent.outputs.result",
		}},
	})

	_, err := BuildDAG(wf)
	if err == nil {
		t.Fatal("expected error for unknown step reference")
	}
	if !strings.Contains(err.Error(), "unknown step") {
		t.Errorf("expected unknown step error, got: %v", err)
	}
}

func TestGetReadySteps(t *testing.T) {
	wf := makeWorkflow(map[string]Step{
		"a": {Name: "a", Definition: "d1", Method: "m1"},
		"b": {Name: "b", Definition: "d2", Method: "m2", Inputs: map[string]string{
			"x": "steps.a.outputs.result",
		}},
		"c": {Name: "c", Definition: "d3", Method: "m3", Inputs: map[string]string{
			"y": "steps.a.outputs.result",
		}},
		"d": {Name: "d", Definition: "d4", Method: "m4", Inputs: map[string]string{
			"z": "steps.b.outputs.result",
		}},
	})

	dag, err := BuildDAG(wf)
	if err != nil {
		t.Fatalf("BuildDAG failed: %v", err)
	}

	// Initially only a is ready
	ready := dag.GetReadySteps(map[string]bool{})
	if len(ready) != 1 || ready[0] != "a" {
		t.Errorf("expected [a], got %v", ready)
	}

	// After a completes, b and c are ready
	ready = dag.GetReadySteps(map[string]bool{"a": true})
	if len(ready) != 2 {
		t.Errorf("expected 2 ready steps, got %d: %v", len(ready), ready)
	}

	// After a and b complete, c and d are ready
	ready = dag.GetReadySteps(map[string]bool{"a": true, "b": true})
	if len(ready) != 2 {
		t.Errorf("expected 2 ready steps (c, d), got %d: %v", len(ready), ready)
	}
}

func TestGetDependencies(t *testing.T) {
	wf := makeWorkflow(map[string]Step{
		"a": {Name: "a", Definition: "d1", Method: "m1"},
		"b": {Name: "b", Definition: "d2", Method: "m2", Inputs: map[string]string{
			"x": "steps.a.outputs.result",
		}},
	})

	dag, err := BuildDAG(wf)
	if err != nil {
		t.Fatalf("BuildDAG failed: %v", err)
	}

	deps := dag.GetDependencies("b")
	if len(deps) != 1 || deps[0] != "a" {
		t.Errorf("expected [a], got %v", deps)
	}

	deps = dag.GetDependencies("a")
	if len(deps) != 0 {
		t.Errorf("expected no deps for a, got %v", deps)
	}
}

func TestConditionBasedDependency(t *testing.T) {
	wf := makeWorkflow(map[string]Step{
		"check": {Name: "check", Definition: "d1", Method: "m1"},
		"act":   {Name: "act", Definition: "d2", Method: "m2", Condition: "steps.check.status.success"},
	})

	dag, err := BuildDAG(wf)
	if err != nil {
		t.Fatalf("BuildDAG failed: %v", err)
	}

	deps := dag.GetDependencies("act")
	if len(deps) != 1 || deps[0] != "check" {
		t.Errorf("expected [check], got %v", deps)
	}
}

func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}
