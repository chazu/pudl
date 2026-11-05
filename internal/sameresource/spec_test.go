package sameresource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParseSpec(t *testing.T, raw string) *Spec {
	t.Helper()
	spec, err := Parse([]byte(raw))
	require.NoError(t, err)
	return spec
}

func TestParseRejectsBadDeclarations(t *testing.T) {
	for name, raw := range map[string]string{
		"not a list":        `{"kind": "k8s.cluster", "key": {"host": "endpoint"}}`,
		"empty list":        `[]`,
		"unknown field":     `[{"kind": "k8s.cluster", "key": {"host": "endpoint"}, "keys": {}}]`,
		"no dot in kind":    `[{"kind": "cluster", "key": {"host": "endpoint"}}]`,
		"upper-case kind":   `[{"kind": "K8s.Cluster", "key": {"host": "endpoint"}}]`,
		"empty key":         `[{"kind": "k8s.cluster", "key": {}}]`,
		"bad part name":     `[{"kind": "k8s.cluster", "key": {"the-host": "endpoint"}}]`,
		"empty path":        `[{"kind": "k8s.cluster", "key": {"host": ""}}]`,
		"unknown value key": `[{"kind": "k8s.cluster", "key": {"host": {"path": "endpoint", "trim": "x"}}}]`,
		"two wildcards":     `[{"kind": "k8s.object", "key": {"a": "xs[*].a", "b": "ys[*].b"}}]`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(raw))
			assert.Error(t, err)
		})
	}
}

func TestKeysReadsEveryDeclaration(t *testing.T) {
	spec := mustParseSpec(t, `[
		{"kind": "k8s.cluster", "key": {"host": "endpoint"}},
		{"kind": "k8s.cluster", "key": {"host": "privateClusterConfig.privateEndpoint"}},
		{"kind": "k8s.cluster", "key": {"host": {"path": "server", "trim_prefix": "https://"}}}
	]`)
	keys := spec.Keys(map[string]any{
		"endpoint":             "34.1.2.3",
		"privateClusterConfig": map[string]any{"privateEndpoint": "10.0.0.2"},
		"server":               "https://34.1.2.3",
	})
	assert.Equal(t, []Key{
		{Kind: "k8s.cluster", Key: `{"host":"10.0.0.2"}`},
		{Kind: "k8s.cluster", Key: `{"host":"34.1.2.3"}`},
	}, keys, "equal keys from two declarations are reported once")
}

func TestKeysNeedEveryPart(t *testing.T) {
	spec := mustParseSpec(t, `[{"kind": "k8s.object", "key": {"cluster": "cluster", "namespace": "metadata.namespace", "name": "metadata.name"}}]`)
	full := spec.Keys(map[string]any{"cluster": "c1", "metadata": map[string]any{"namespace": "default", "name": "web"}})
	assert.Equal(t, []Key{{Kind: "k8s.object", Key: `{"cluster":"c1","name":"web","namespace":"default"}`}}, full)

	assert.Empty(t, spec.Keys(map[string]any{"cluster": "c1", "metadata": map[string]any{"name": "web"}}),
		"a missing part yields no key, never a partial one")
	assert.Empty(t, spec.Keys(map[string]any{"cluster": "", "metadata": map[string]any{"namespace": "default", "name": "web"}}),
		"an empty value yields no key")
	assert.Empty(t, spec.Keys(map[string]any{"cluster": map[string]any{}, "metadata": map[string]any{"namespace": "default", "name": "web"}}),
		"a non-scalar value yields no key")
}

func TestKeysFanOutOverOneWildcard(t *testing.T) {
	spec := mustParseSpec(t, `[{"kind": "k8s.cluster", "key": {"host": {"path": "endpoints[*]", "last_segment": true}}}]`)
	keys := spec.Keys(map[string]any{"endpoints": []any{"https://a/b/1.1.1.1", "2.2.2.2", "", map[string]any{}}})
	assert.Equal(t, []Key{
		{Kind: "k8s.cluster", Key: `{"host":"1.1.1.1"}`},
		{Kind: "k8s.cluster", Key: `{"host":"2.2.2.2"}`},
	}, keys)
}

func TestCanonicalChangesWithTheSpec(t *testing.T) {
	a := mustParseSpec(t, `[{"kind": "k8s.cluster", "key": {"host": "endpoint"}}]`)
	b := mustParseSpec(t, `[{"kind": "k8s.cluster", "key": {"host": {"path": "endpoint", "prepend": "x"}}}]`)
	assert.NotEqual(t, a.Canonical(), b.Canonical())
	assert.Equal(t, a.Canonical(), mustParseSpec(t, `[{"key": {"host": "endpoint"}, "kind": "k8s.cluster"}]`).Canonical())
	assert.Equal(t, "", (*Spec)(nil).Canonical())
}
