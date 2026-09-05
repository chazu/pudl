package factstore_test

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/chazu/pudl/pkg/eval"
	"github.com/chazu/pudl/pkg/factstore"
	"github.com/stretchr/testify/require"
)

func TestQueryNumericJoinsAndClosure(t *testing.T) {
	s := openStore(t)
	addFact(t, s, "edge", `{"from":9007199254740992,"to":9007199254740993}`)
	addFact(t, s, "edge", `{"from":9007199254740993,"to":9007199254740994}`)
	rules, err := eval.ParseRulesFromSource(`
base: {head: {rel: "reach", args: {from: "$X", to: "$Y"}}, body: [{rel: "edge", args: {from: "$X", to: "$Y"}}]}
step: {head: {rel: "reach", args: {from: "$X", to: "$Z"}}, body: [
 {rel: "reach", args: {from: "$X", to: "$Y"}}, {rel: "reach", args: {from: "$Y", to: "$Z"}},
]}
selected: {head: {rel: "selected", args: {to: "$Y"}}, body: [{rel: "edge", args: {from: 9.007199254740993e15, to: "$Y"}}]}
larger: {head: {rel: "larger", args: {to: "$Y"}}, body: [{rel: "edge", args: {from: ">9007199254740992", to: "$Y"}}]}
`)
	require.NoError(t, err)
	for _, opts := range numericScopes() {
		opts.Rules, opts.Relation = rules, "reach"
		rows, err := s.Query(opts)
		require.NoError(t, err)
		var pairs []string
		for _, row := range rows {
			pairs = append(pairs, fmt.Sprintf("%v->%v", row.Args["from"], row.Args["to"]))
		}
		require.ElementsMatch(t, []string{"9007199254740992->9007199254740993", "9007199254740992->9007199254740994", "9007199254740993->9007199254740994"}, pairs)
		for _, rel := range []string{"selected", "larger"} {
			opts.Relation = rel
			rows, err := s.Query(opts)
			require.NoError(t, err)
			require.Equal(t, []factstore.Tuple{{Relation: rel, Args: map[string]interface{}{"to": int64(9007199254740994)}}}, rows)
		}
	}
}

func TestQueryRejectsUnsupportedNumericOperands(t *testing.T) {
	s := openStore(t)
	for _, bound := range []interface{}{json.Number("9223372036854775808"), json.Number("0.123456789012345678901"), json.Number("bad"), uint64(math.MaxUint64), math.NaN(), math.Inf(1)} {
		for _, rel := range []string{"numbers", "projected", "recursive"} {
			_, err := s.Query(factstore.QueryOptions{Relation: rel, Rules: numericRules(t), Constraints: map[string]interface{}{"n": bound}})
			require.ErrorContains(t, err, "unsupported query number", "%s %v", rel, bound)
		}
	}
	for _, literal := range []string{"9223372036854775808", "0.123456789012345678901", `">9223372036854775808"`} {
		rules, err := eval.ParseRulesFromSource(`r: {head: {rel: "r", args: {label: "$L"}}, body: [{rel: "numbers", args: {label: "$L", n: ` + literal + `}}]}`)
		require.NoError(t, err)
		require.Len(t, rules, 1, "invalid numeric operands must not silently drop the rule")
		_, err = s.Query(factstore.QueryOptions{Relation: "r", Rules: rules})
		require.ErrorContains(t, err, "unsupported query number")
	}
}

func TestQueryFractionalAndNestedNumbers(t *testing.T) {
	s := openStore(t)
	for _, raw := range []string{`{"n":0.1}`, `{"n":0.10000000000000002}`} {
		addFact(t, s, "numbers", raw)
	}
	for _, rel := range []string{"numbers", "projected", "recursive"} {
		for _, opts := range numericScopes() {
			opts.Relation, opts.Rules = rel, numericRules(t)
			opts.Constraints = map[string]interface{}{"n": json.Number("0.10000000000000002")}
			rows, err := s.Query(opts)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			encoded, err := json.Marshal(rows[0].Args)
			require.NoError(t, err)
			require.Equal(t, `{"n":0.10000000000000002}`, string(encoded))
		}
	}
	addFact(t, s, "nested", `{"values":[9007199254740993,{"min":-9223372036854775808}]}`)
	rows, err := s.Query(factstore.QueryOptions{Relation: "nested"})
	require.NoError(t, err)
	encoded, err := json.Marshal(rows[0].Args)
	require.NoError(t, err)
	require.Equal(t, `{"values":[9007199254740993,{"min":-9223372036854775808}]}`, string(encoded))
}
