package projection

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// rewriteValues projects doc through one relation and returns the values of
// its arg "v", sorted.
func rewriteValues(t *testing.T, relation, doc string) []string {
	t.Helper()
	rels, err := ParseRelations(json.RawMessage(`{"r": ` + relation + `}`))
	require.NoError(t, err)
	var data any
	require.NoError(t, json.Unmarshal([]byte(doc), &data))
	res := Compute(&SchemaSpec{Schema: "x.#T", Relations: rels}, "e", "r", data)
	var out []string
	for _, f := range res.Facts {
		var args map[string]any
		require.NoError(t, json.Unmarshal([]byte(f.Args), &args))
		out = append(out, args["v"].(string))
	}
	sort.Strings(out)
	return out
}

func TestRewriteTakesTheFirstMatchingRule(t *testing.T) {
	rel := `{"args": {"v": {"path": "key", "rewrite": [
		{"match": "^(.*)/cryptoKeyVersions/[0-9]+$", "replace": "$1"},
		{"match": "^(projects/.*)$", "replace": "$1"}]}}}`
	require.Equal(t, []string{"projects/p/keyRings/r/cryptoKeys/k"},
		rewriteValues(t, rel, `{"key": "projects/p/keyRings/r/cryptoKeys/k/cryptoKeyVersions/3"}`))
	require.Equal(t, []string{"projects/p/keyRings/r/cryptoKeys/k"},
		rewriteValues(t, rel, `{"key": "projects/p/keyRings/r/cryptoKeys/k"}`), "the second rule")
}

func TestRewriteWithoutAMatchYieldsNoFact(t *testing.T) {
	rel := `{"args": {"v": {"path": "sa", "rewrite": [{"match": "^([^@]+@[^@]+)$", "replace": "$1"}]}, "other": "x"}}`
	require.Empty(t, rewriteValues(t, rel, `{"sa": "default", "x": "kept?"}`), "the whole row is dropped, not just the arg")
	require.Empty(t, rewriteValues(t, rel, `{"sa": 7}`), "a non-string matches nothing")
	require.Empty(t, rewriteValues(t, rel, `{"x": "no sa field"}`), "an absent value yields no fact either, unlike a plain path arg")
	require.Equal(t, []string{"ci@p.iam.gserviceaccount.com"}, rewriteValues(t, rel, `{"sa": "ci@p.iam.gserviceaccount.com"}`))
}

func TestRewriteOnAWildcardSkipsOnlyUnmatchedValues(t *testing.T) {
	rel := `{"args": {"v": {"path": "images[*]", "rewrite": [
		{"match": "^gcr\\.io/([^/]+)/([^:@]+)", "replace": "us-docker.pkg.dev/${1}/gcr.io/${2}"},
		{"match": "^([a-z0-9-]+-docker\\.pkg\\.dev/[^:@]+)", "replace": "$1"}]}}}`
	got := rewriteValues(t, rel, `{"images": ["gcr.io/p/api:v1", "us-central1-docker.pkg.dev/p/apps/web@sha256:ab", "nginx:1.27"]}`)
	require.Equal(t, []string{"us-central1-docker.pkg.dev/p/apps/web", "us-docker.pkg.dev/p/gcr.io/api"}, got)
}

func TestRewriteIsValidated(t *testing.T) {
	for _, bad := range []string{
		`{"args": {"v": {"path": "x", "rewrite": []}}}`,
		`{"args": {"v": {"path": "x", "rewrite": [{"replace": "$1"}]}}}`,
		`{"args": {"v": {"path": "x", "rewrite": [{"match": "(", "replace": "$1"}]}}}`,
		`{"args": {"v": {"exists": "x", "rewrite": [{"match": "a", "replace": "b"}]}}}`,
	} {
		_, err := ParseRelations(json.RawMessage(`{"r": ` + bad + `}`))
		require.Error(t, err, bad)
	}
}

func TestRewriteChangesTheFingerprintOnlyWhenSet(t *testing.T) {
	plain, err := ParseRelations(json.RawMessage(`{"r": {"args": {"v": {"path": "x"}}}}`))
	require.NoError(t, err)
	rewritten, err := ParseRelations(json.RawMessage(`{"r": {"args": {"v": {"path": "x", "rewrite": [{"match": "a", "replace": "b"}]}}}}`))
	require.NoError(t, err)
	require.NotEqual(t, fingerprint(&SchemaSpec{Relations: plain}), fingerprint(&SchemaSpec{Relations: rewritten}))
}
