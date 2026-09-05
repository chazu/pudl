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

func numericRules(t *testing.T) []factstore.Rule {
	t.Helper()
	rules, err := eval.ParseRulesFromSource(`
project: {
 head: {rel: "projected", args: {n: "$N"}}
 body: [{rel: "numbers", args: {n: "$N"}}]
}
base: {
 head: {rel: "recursive", args: {n: "$N"}}
 body: [{rel: "numbers", args: {n: "$N"}}]
}
step: {
 head: {rel: "recursive", args: {n: "$N"}}
 body: [{rel: "recursive", args: {n: "$N"}}]
}`)
	require.NoError(t, err)
	return rules
}

func numericScopes() []factstore.QueryOptions {
	at := int64(1)
	return []factstore.QueryOptions{{}, {ValidAt: &at}, {TxAt: &at}, {ValidAt: &at, TxAt: &at}}
}

func TestQueryPreservesNumericValuesAndEquality(t *testing.T) {
	s := openStore(t)
	values := []int64{9007199254740992, 9007199254740993, -9007199254740992, -9007199254740993, math.MinInt64, math.MaxInt64}
	for _, n := range values {
		addFact(t, s, "numbers", fmt.Sprintf(`{"n":%d}`, n))
	}
	rules := numericRules(t)
	for _, rel := range []string{"numbers", "projected", "recursive"} {
		for mode, opts := range numericScopes() {
			t.Run(fmt.Sprintf("%s/scope%d", rel, mode), func(t *testing.T) {
				opts.Relation, opts.Rules = rel, rules
				rows, err := s.Query(opts)
				require.NoError(t, err)
				var got []int64
				for _, row := range rows {
					require.IsType(t, int64(0), row.Args["n"])
					got = append(got, row.Args["n"].(int64))
				}
				require.ElementsMatch(t, values, got)
				for _, n := range values {
					for _, bound := range []interface{}{n, json.Number(fmt.Sprint(n))} {
						opts.Constraints = map[string]interface{}{"n": bound}
						rows, err = s.Query(opts)
						require.NoError(t, err)
						require.Len(t, rows, 1)
						data, err := json.Marshal(rows[0].Args)
						require.NoError(t, err)
						require.Equal(t, fmt.Sprintf(`{"n":%d}`, n), string(data))
					}
				}
			})
		}
	}
}

func TestQueryEquivalentNumericSpellings(t *testing.T) {
	for _, pair := range [][2]string{{"9007199254740993", "9.007199254740993e15"}, {"1", "1.000e0"}, {"0.1", "1e-1"}} {
		s := openStore(t)
		addFact(t, s, "numbers", `{"n":`+pair[0]+`}`)
		addFact(t, s, "selected", `{"n":`+pair[1]+`}`)
		rules, err := eval.ParseRulesFromSource(`
joined: {head: {rel: "joined", args: {n: "$N"}}, body: [
 {rel: "numbers", args: {n: "$N"}}, {rel: "selected", args: {n: "$N"}},
]}
ground: {head: {rel: "ground", args: {n: "$N"}}, body: [
 {rel: "numbers", args: {n: "$N"}}, {rel: "selected", args: {n: ` + pair[1] + `}},
]}`)
		require.NoError(t, err)
		for _, rel := range []string{"numbers", "joined", "ground"} {
			for _, opts := range numericScopes() {
				opts.Relation, opts.Rules = rel, rules
				opts.Constraints = map[string]interface{}{"n": json.Number(pair[1])}
				rows, err := s.Query(opts)
				require.NoError(t, err, "%s %v", rel, pair)
				require.Len(t, rows, 1, "%s %v", rel, pair)
			}
		}
	}
}

func TestQueryRejectsNumbersThatCannotBeRepresented(t *testing.T) {
	for _, number := range []string{"9223372036854775808", "-9223372036854775809", "0.123456789012345678901", "1e100000000000000000000", "1e-100000000000000000000"} {
		t.Run(number, func(t *testing.T) {
			s := openStore(t)
			addFact(t, s, "numbers", `{"n":`+number+`}`)
			for _, rel := range []string{"numbers", "projected", "recursive"} {
				for _, opts := range numericScopes() {
					opts.Relation, opts.Rules = rel, numericRules(t)
					rows, err := s.Query(opts)
					require.ErrorContains(t, err, "unsupported query number", rel)
					require.Empty(t, rows)
				}
			}
			facts, err := s.QueryFacts(factstore.FactFilter{Relation: "numbers"})
			require.NoError(t, err)
			require.Equal(t, `{"n":`+number+`}`, facts[0].Args, "raw evidence remains available")
		})
	}
}
