package cmd

import (
	"sort"
	"testing"

	"github.com/chazu/pudl/internal/systemmodel"
)

// nilIdentity is the no-schema resolver (forces the name|path|id + nested-name
// path, the k8s/inventory shape used in these tests).
func nilIdentity(string) []string { return nil }

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestProducedIdentities_NestedAndTopLevel(t *testing.T) {
	// k8s-style nested metadata.name
	ns := []map[string]any{{
		"apiVersion": "v1", "kind": "Namespace",
		"metadata": map[string]any{"name": "foo"},
	}}
	got := keys(producedIdentities(ns, nilIdentity))
	if len(got) != 1 || got[0] != "foo" {
		t.Fatalf("nested name: got %v, want [foo]", got)
	}

	// inventory-style top-level name
	pkg := []map[string]any{{"_schema": "pudl/linux.#Package", "name": "htop"}}
	got = keys(producedIdentities(pkg, nilIdentity))
	if len(got) != 1 || got[0] != "htop" {
		t.Fatalf("top-level name: got %v, want [htop]", got)
	}
}

func TestReferencedValues_RecursiveLeaves(t *testing.T) {
	dep := []map[string]any{{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "nginx", "namespace": "foo"},
	}}
	got := referencedValues(dep)
	// real reference values are collected...
	for _, want := range []string{"nginx", "foo"} {
		if _, ok := got[want]; !ok {
			t.Errorf("referencedValues missing %q (got %v)", want, keys(got))
		}
	}
	// ...but structural type tags must NOT be (they'd mint spurious edges).
	for _, no := range []string{"apps/v1", "Deployment"} {
		if _, ok := got[no]; ok {
			t.Errorf("referencedValues must skip structural tag %q (got %v)", no, keys(got))
		}
	}
}

func TestProducedIdentities_IgnoresNonMetadataName(t *testing.T) {
	// container "name" is NOT a produced resource identity; only metadata.name is.
	dep := []map[string]any{{
		"kind":     "Deployment",
		"metadata": map[string]any{"name": "web"},
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
			"containers": []any{map[string]any{"name": "sidecar", "image": "x"}},
		}}},
	}}
	got := producedIdentities(dep, nilIdentity)
	if _, ok := got["web"]; !ok {
		t.Errorf("expected metadata.name 'web' (got %v)", keys(got))
	}
	if _, ok := got["sidecar"]; ok {
		t.Errorf("container name 'sidecar' must NOT be a produced identity (got %v)", keys(got))
	}
}

func model(name string, dependsOn []string, desired []map[string]any) *systemmodel.SystemModel {
	return &systemmodel.SystemModel{Name: name, DependsOn: dependsOn, Desired: desired}
}

func TestDeriveDependencies_NamespaceReference(t *testing.T) {
	network := model("network", nil, []map[string]any{{
		"kind": "Namespace", "metadata": map[string]any{"name": "foo"},
	}})
	workloads := model("workloads", nil, []map[string]any{{
		"kind": "Deployment", "metadata": map[string]any{"name": "nginx", "namespace": "foo"},
	}})

	got := deriveDependencies([]*systemmodel.SystemModel{network, workloads}, nilIdentity)

	// workloads references namespace "foo" which network produces -> workloads -> network
	if deps, ok := got["workloads"]; !ok || len(deps) != 1 {
		t.Fatalf("expected workloads->{network}, got %v", got)
	} else if _, ok := deps["network"]; !ok {
		t.Fatalf("expected workloads->network, got %v", keys(deps))
	}
	// network references nothing workloads produces -> no edge
	if _, ok := got["network"]; ok {
		t.Fatalf("network should derive no deps, got %v", got["network"])
	}
}

func TestDeriveDependencies_SkipsDeclared(t *testing.T) {
	network := model("network", nil, []map[string]any{{
		"kind": "Namespace", "metadata": map[string]any{"name": "foo"},
	}})
	// workloads already DECLARES network -> derivation must not duplicate it
	workloads := model("workloads", []string{"network"}, []map[string]any{{
		"kind": "Deployment", "metadata": map[string]any{"name": "nginx", "namespace": "foo"},
	}})

	got := deriveDependencies([]*systemmodel.SystemModel{network, workloads}, nilIdentity)
	if _, ok := got["workloads"]; ok {
		t.Fatalf("declared edge should not be re-derived, got %v", got["workloads"])
	}
}

func TestDeriveDependencies_NoSelfEdge(t *testing.T) {
	// A model that references its own resource name must not derive a self-edge.
	solo := model("solo", nil, []map[string]any{{
		"kind": "Service", "metadata": map[string]any{"name": "svc"},
		"spec": map[string]any{"selector": map[string]any{"app": "svc"}},
	}})
	got := deriveDependencies([]*systemmodel.SystemModel{solo}, nilIdentity)
	if len(got) != 0 {
		t.Fatalf("expected no edges, got %v", got)
	}
}
