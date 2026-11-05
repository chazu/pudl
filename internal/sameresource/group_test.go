package sameresource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func namespaces(m map[string]string) func(string) string {
	return func(id string) string { return m[id] }
}

func TestBuildGroupsTransitively(t *testing.T) {
	links := []Link{
		{A: "gke", B: "kctx", Kind: "k8s.cluster"},
		{A: "kctx", B: "gke", Kind: "k8s.cluster"},
		{A: "kctx", B: "kctx", Kind: "k8s.cluster"}, // self rows from the built-in rule
		{A: "tf", B: "gke", Kind: "k8s.cluster"},
		{A: "dep-gcp", B: "dep-k8s", Kind: "k8s.object"},
		{A: "lonely", B: "lonely", Kind: "k8s.object"},
	}
	ns := namespaces(map[string]string{
		"gke": "gcp.#GKECluster", "kctx": "k8s.#Cluster", "tf": "tf.#Resource",
		"dep-gcp": "gcp.#K8sDeployment", "dep-k8s": "k8s.#Deployment",
	})
	groups, conflicts := Build(links, ns)
	assert.Empty(t, conflicts)
	assert.Equal(t, []Group{
		{ID: "dep-gcp", Members: []string{"dep-gcp", "dep-k8s"}, Kinds: []string{"k8s.object"}},
		{ID: "gke", Members: []string{"gke", "kctx", "tf"}, Kinds: []string{"k8s.cluster"}},
	}, groups, "self links form no group; the ID is the smallest member")
}

func TestBuildRefusesTwoMembersOfOneNamespace(t *testing.T) {
	// Two GKE cluster records share an endpoint with one kube context: the
	// old cluster and its replacement, say. Linking all three would claim
	// two GCP clusters are one, so the component is not formed.
	links := []Link{
		{A: "gke-old", B: "kctx", Kind: "k8s.cluster"},
		{A: "gke-new", B: "kctx", Kind: "k8s.cluster"},
		{A: "a", B: "b", Kind: "k8s.object"},
	}
	ns := namespaces(map[string]string{"gke-old": "gcp.#GKECluster", "gke-new": "gcp.#GKECluster", "kctx": "k8s.#Cluster", "a": "x.#A", "b": "y.#B"})
	groups, conflicts := Build(links, ns)
	assert.Equal(t, []Group{{ID: "a", Members: []string{"a", "b"}, Kinds: []string{"k8s.object"}}}, groups)
	assert.Equal(t, []Conflict{{
		Members:    []string{"gke-new", "gke-old", "kctx"},
		Kinds:      []string{"k8s.cluster"},
		Namespaces: map[string][]string{"gcp.#GKECluster": {"gke-new", "gke-old"}},
	}}, conflicts)
}

func TestBuildUnknownNamespaceNeverConflicts(t *testing.T) {
	groups, conflicts := Build([]Link{{A: "a", B: "b", Kind: "k8s.object"}}, nil)
	assert.Empty(t, conflicts)
	assert.Equal(t, []Group{{ID: "a", Members: []string{"a", "b"}, Kinds: []string{"k8s.object"}}}, groups)
}
