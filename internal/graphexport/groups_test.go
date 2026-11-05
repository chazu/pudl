package graphexport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/resourcegroup"
	"github.com/chazu/pudl/internal/sameresource"
)

func TestMergeGroupsMakesOneResourcePerGroup(t *testing.T) {
	items := []builtResource{
		{r: Resource{ID: "r-k8s", Schema: "k8s.#Deployment", Type: "k8s.deployment", Name: "web", EntryID: "e1",
			Evidence: []Evidence{{Basis: "observed", Source: "kubectl"}}}, refs: []string{"prod/default/web"}, origin: "kubectl"},
		{r: Resource{ID: "r-gcp", Schema: "gcp.#K8sDeployment", Type: "gcp.k8s.deployment", Name: "web", EntryID: "e2", Account: "example-project",
			Evidence: []Evidence{{Basis: "observed", Source: "cai"}}}, refs: []string{"//container.googleapis.com/x"}, origin: "gcp"},
		{r: Resource{ID: "r-other", Schema: "k8s.#Deployment", Type: "k8s.deployment", Name: "api"}},
	}
	idx := resourcegroup.FromLinks([]sameresource.Link{{A: "r-k8s", B: "r-gcp", Kind: "k8s.object"}}, nil)
	out, aliases := mergeGroups(items, idx)

	require.Len(t, out, 2)
	assert.Equal(t, map[string]string{"r-k8s": "r-gcp", "r-gcp": "r-gcp"}, aliases)
	var merged builtResource
	for _, it := range out {
		if it.r.ID == "r-gcp" {
			merged = it
		}
	}
	assert.Equal(t, "gcp.k8s.deployment", merged.r.Type, "labels come from the first schema by name")
	assert.Equal(t, "example-project", merged.r.Account)
	assert.Equal(t, []Representation{
		{ID: "r-gcp", Schema: "gcp.#K8sDeployment", Type: "gcp.k8s.deployment", EntryID: "e2", Origin: "gcp"},
		{ID: "r-k8s", Schema: "k8s.#Deployment", Type: "k8s.deployment", EntryID: "e1", Origin: "kubectl"},
	}, merged.r.Representations)
	assert.Len(t, merged.r.Evidence, 2)
	assert.ElementsMatch(t, []string{"//container.googleapis.com/x", "prod/default/web"}, merged.refs)

	index := newRefIndex()
	for _, it := range out {
		index.add(it.r.ID, it.refs)
	}
	for m, g := range aliases {
		index.alias(m, g)
	}
	for _, ref := range []string{"r-k8s", "prod/default/web", "//container.googleapis.com/x"} {
		id, problem := index.resolve(ref)
		assert.Equal(t, "r-gcp", id, ref)
		assert.Empty(t, problem)
	}
}

func TestMergeGroupsWithoutGroupsChangesNothing(t *testing.T) {
	items := []builtResource{{r: Resource{ID: "a"}}}
	out, aliases := mergeGroups(items, nil)
	assert.Equal(t, items, out)
	assert.Empty(t, aliases)
}
