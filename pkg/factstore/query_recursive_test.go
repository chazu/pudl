package factstore_test

import (
	"fmt"
	"testing"

	"github.com/chazu/pudl/pkg/eval"
	"github.com/chazu/pudl/pkg/factstore"
	"github.com/stretchr/testify/require"
)

func TestNonlinearRecursiveQueryReachesFixedPoint(t *testing.T) {
	for _, cycle := range []bool{false, true} {
		t.Run(fmt.Sprintf("cycle=%t", cycle), func(t *testing.T) {
			s := openStore(t)
			for _, edge := range []string{`{"from":"a","to":"b"}`, `{"from":"b","to":"c"}`, `{"from":"c","to":"d"}`} {
				addFact(t, s, "edge", edge)
			}
			if cycle {
				addFact(t, s, "edge", `{"from":"d","to":"a"}`)
			}
			rules, err := eval.ParseRulesFromSource(`
base: {
	head: {rel: "reach", args: {from: "$X", to: "$Y"}}
	body: [{rel: "edge", args: {from: "$X", to: "$Y"}}]
}

step: {
	head: {rel: "reach", args: {from: "$X", to: "$Z"}}
	body: [
		{rel: "reach", args: {from: "$X", to: "$Y"}},
		{rel: "reach", args: {from: "$Y", to: "$Z"}},
	]
}`)
			require.NoError(t, err)
			var want []string
			for i, from := range []string{"a", "b", "c", "d"} {
				for j, to := range []string{"a", "b", "c", "d"} {
					if cycle || i < j {
						want = append(want, from+"->"+to)
					}
				}
			}
			for _, historical := range []bool{false, true} {
				opts := factstore.QueryOptions{Relation: "reach", Rules: rules}
				if historical {
					at := int64(1)
					opts.ValidAt, opts.TxAt = &at, &at
				}
				rows, err := s.Query(opts)
				require.NoError(t, err)
				var got []string
				for _, row := range rows {
					got = append(got, fmt.Sprintf("%v->%v", row.Args["from"], row.Args["to"]))
				}
				require.ElementsMatch(t, want, got, "historical=%t", historical)
			}
		})
	}
}

func TestDerivedJoinUsesInputsFromDifferentRounds(t *testing.T) {
	s := openStore(t)
	addFact(t, s, "left_source", `{"value":"a"}`)
	addFact(t, s, "right_source", `{"value":"b"}`)
	rules, err := eval.ParseRulesFromSource(`
left: {
	head: {rel: "left", args: {value: "$V"}}
	body: [{rel: "left_source", args: {value: "$V"}}]
}
middle: {
	head: {rel: "middle", args: {value: "$V"}}
	body: [{rel: "right_source", args: {value: "$V"}}]
}
right: {
	head: {rel: "right", args: {value: "$V"}}
	body: [{rel: "middle", args: {value: "$V"}}]
}
joined: {
	head: {rel: "joined", args: {left: "$L", right: "$R"}}
	body: [
		{rel: "left", args: {value: "$L"}},
		{rel: "right", args: {value: "$R"}},
	]
}`)
	require.NoError(t, err)
	rows, err := s.Query(factstore.QueryOptions{Relation: "joined", Rules: rules})
	require.NoError(t, err)
	require.Equal(t, []factstore.Tuple{{Relation: "joined", Args: map[string]interface{}{
		"left": "a", "right": "b",
	}}}, rows)
}
