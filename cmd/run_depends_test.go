package cmd

import (
	"reflect"
	"testing"
)

func set(keys ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return m
}

func TestDependencyDiff_Idempotent(t *testing.T) {
	// Declared == current → no add, no invalidate (re-run is a no-op; this is
	// what prevents per-run fact churn).
	declared := set("network", "dns")
	current := map[string]string{"network": "id1", "dns": "id2"}
	add, inv := dependencyDiff(declared, current)
	if len(add) != 0 || len(inv) != 0 {
		t.Fatalf("expected no-op, got add=%v invalidate=%v", add, inv)
	}
}

func TestDependencyDiff_AddNew(t *testing.T) {
	declared := set("network", "dns")
	current := map[string]string{"network": "id1"}
	add, inv := dependencyDiff(declared, current)
	if !reflect.DeepEqual(add, []string{"dns"}) {
		t.Fatalf("expected add=[dns], got %v", add)
	}
	if len(inv) != 0 {
		t.Fatalf("expected no invalidate, got %v", inv)
	}
}

func TestDependencyDiff_InvalidateRemoved(t *testing.T) {
	// dns dropped from the declaration → its fact id must be invalidated so a
	// stale edge can't pollute blast-radius answers.
	declared := set("network")
	current := map[string]string{"network": "id1", "dns": "id2"}
	add, inv := dependencyDiff(declared, current)
	if len(add) != 0 {
		t.Fatalf("expected no add, got %v", add)
	}
	if !reflect.DeepEqual(inv, []string{"id2"}) {
		t.Fatalf("expected invalidate=[id2], got %v", inv)
	}
}

func TestDependencyDiff_AddAndRemove(t *testing.T) {
	declared := set("network", "vpc")
	current := map[string]string{"network": "id1", "dns": "id2"}
	add, inv := dependencyDiff(declared, current)
	if !reflect.DeepEqual(add, []string{"vpc"}) {
		t.Fatalf("expected add=[vpc], got %v", add)
	}
	if !reflect.DeepEqual(inv, []string{"id2"}) {
		t.Fatalf("expected invalidate=[id2], got %v", inv)
	}
}

func TestEdgeArgs(t *testing.T) {
	from, to := edgeArgs(`{"from":"compute","to":"network"}`)
	if from != "compute" || to != "network" {
		t.Fatalf("got from=%q to=%q", from, to)
	}
	// Malformed args yield empty strings, not a panic.
	if f, tt := edgeArgs(`not json`); f != "" || tt != "" {
		t.Fatalf("expected empty for bad json, got %q/%q", f, tt)
	}
}
