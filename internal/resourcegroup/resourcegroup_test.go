package resourcegroup

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/sameresource"
	"github.com/chazu/pudl/internal/validator"
)

func TestNamespaceIsTheRootOfTheBaseSchemaChain(t *testing.T) {
	meta := map[string]validator.SchemaMetadata{
		"gcp.#AutopilotCluster": {BaseSchema: "gcp.#GKECluster"},
		"gcp.#GKECluster":       {},
		"x.#Loop":               {BaseSchema: "x.#Loop"},
	}
	lookup := func(s string) (validator.SchemaMetadata, bool) { m, ok := meta[s]; return m, ok }
	assert.Equal(t, "gcp.#GKECluster", Namespace("gcp.#AutopilotCluster", lookup))
	assert.Equal(t, "gcp.#GKECluster", Namespace("gcp.#GKECluster", lookup))
	assert.Equal(t, "y.#Unknown", Namespace("y.#Unknown", lookup))
	assert.Equal(t, "x.#Loop", Namespace("x.#Loop", lookup), "a cycle ends the walk")
	assert.Equal(t, "", Namespace("", lookup))
}

func TestGroupOf(t *testing.T) {
	x := FromLinks([]sameresource.Link{{A: "b", B: "a", Kind: "k8s.object"}}, nil)
	g, ok := x.GroupOf("b")
	assert.True(t, ok)
	assert.Equal(t, "a", g.ID)
	_, ok = x.GroupOf("c")
	assert.False(t, ok)
	_, ok = (*Index)(nil).GroupOf("a")
	assert.False(t, ok)
}
