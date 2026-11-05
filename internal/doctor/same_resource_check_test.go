package doctor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/resourcegroup"
	"github.com/chazu/pudl/internal/sameresource"
)

func TestCheckSameResource(t *testing.T) {
	declared := map[string]bool{"k8s.cluster": true}
	groups := resourcegroup.FromLinks([]sameresource.Link{{A: "a", B: "b", Kind: "k8s.cluster"}}, nil)

	ok := CheckSameResource(SameResourceInput{DeclaredKinds: declared, UsedKinds: map[string][]string{"k8s.cluster": {"schema x.#A"}}, Groups: groups})
	assert.Equal(t, "ok", ok.Status)
	assert.Equal(t, "Shared identity: 1 group(s), 1 kind(s) declared", ok.Message)

	undeclared := CheckSameResource(SameResourceInput{DeclaredKinds: declared, UsedKinds: map[string][]string{"aws.cluster": {"schema aws.#EKS", "rule r"}}})
	assert.Equal(t, "warning", undeclared.Status)
	assert.Contains(t, undeclared.Details, "kind aws.cluster is not declared by a same_resource_kind fact (named by schema aws.#EKS, rule r)")

	ns := map[string]string{"old": "gcp.#GKE", "new": "gcp.#GKE", "ctx": "k8s.#Cluster"}
	conflict := resourcegroup.FromLinks([]sameresource.Link{{A: "old", B: "ctx", Kind: "k8s.cluster"}, {A: "new", B: "ctx", Kind: "k8s.cluster"}},
		func(id string) string { return ns[id] })
	conflicted := CheckSameResource(SameResourceInput{DeclaredKinds: declared, Groups: conflict})
	assert.Equal(t, "warning", conflicted.Status)
	assert.Contains(t, conflicted.Details, "k8s.cluster records ctx, new, old share a key but were not grouped: more than one record of gcp.#GKE (new, old)")

	failed := CheckSameResource(SameResourceInput{Err: errors.New("boom")})
	assert.Equal(t, "warning", failed.Status)
	assert.Equal(t, "boom", failed.Details)
}
