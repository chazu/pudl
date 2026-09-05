package factstore_test

import (
	"testing"

	"github.com/chazu/pudl/pkg/eval"
	"github.com/chazu/pudl/pkg/factstore"
	"github.com/stretchr/testify/require"
)

func TestDerivedQueryAppliesEveryConstraint(t *testing.T) {
	s := openStore(t)
	for _, args := range []string{`{"a":1,"b":2}`, `{"a":1,"b":3}`, `{"a":2,"b":2}`} {
		addFact(t, s, "source", args)
	}
	rules, err := eval.ParseRulesFromSource(`
project: {
	head: {rel: "project", args: {a: "$A", b: "$B"}}
	body: [{rel: "source", args: {a: "$A", b: "$B"}}]
}`)
	require.NoError(t, err)
	// Repeated calls exercise Go's independently randomized map traversal. A
	// row satisfying only one predicate must never slip into the conjunction.
	for i := range 128 {
		rows, err := s.Query(factstore.QueryOptions{
			Relation: "project", Rules: rules,
			Constraints: map[string]interface{}{"a": 1, "b": 2},
		})
		require.NoError(t, err)
		require.Equal(t, []factstore.Tuple{{Relation: "project", Args: map[string]interface{}{
			"a": int64(1), "b": int64(2),
		}}}, rows, "query %d must enforce both constraints", i)
	}
}
