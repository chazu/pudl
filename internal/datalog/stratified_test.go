package datalog

import (
	"fmt"
	"strings"
	"testing"
)

func TestCountOverNonCyclicDerivedRelation(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "svc", `{"id":"a","team":"x","status":"failed"}`)
	addTestFact(t, db, "svc", `{"id":"b","team":"x","status":"failed"}`)
	addTestFact(t, db, "svc", `{"id":"c","team":"y","status":"ok"}`)
	rules := mustParse(t, `
failed: {
	head: {rel: "failed", args: {id: "$S", team: "$T"}}
	body: [{rel: "svc", args: {id: "$S", team: "$T", status: "failed"}}]
}
failures_per_team: {
	head: {rel: "failures_per_team", args: {team: "$T", n: "count($S)"}}
	body: [{rel: "failed", args: {id: "$S", team: "$T"}}]
}
`)
	got, err := Evaluate(db, rules, "failures_per_team", nil, TemporalScope{})
	if err != nil {
		t.Fatalf("count over a derived relation: %v", err)
	}
	if len(got) != 1 || got[0].Args["team"] != "x" || got[0].Args["n"] != int64(2) {
		t.Fatalf("want failures_per_team(team=x, n=2), got %+v", got)
	}
}

func TestCountOverDerivedRelationBesideRecursion(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "edge", `{"from":"a","to":"b"}`)
	addTestFact(t, db, "edge", `{"from":"b","to":"c"}`)
	rules := mustParse(t, `
reach_base: {
	head: {rel: "reach", args: {from: "$A", to: "$B"}}
	body: [{rel: "edge", args: {from: "$A", to: "$B"}}]
}
reach_rec: {
	head: {rel: "reach", args: {from: "$A", to: "$C"}}
	body: [{rel: "edge", args: {from: "$A", to: "$B"}}, {rel: "reach", args: {from: "$B", to: "$C"}}]
}
fanout: {
	head: {rel: "fanout", args: {from: "$A", n: "count($B)"}}
	body: [{rel: "reach", args: {from: "$A", to: "$B"}}]
}
`)
	got, err := Evaluate(db, rules, "fanout", map[string]interface{}{"from": "a"}, TemporalScope{})
	if err != nil {
		t.Fatalf("count over a recursive relation's result: %v", err)
	}
	if len(got) != 1 || got[0].Args["n"] != int64(2) {
		t.Fatalf("want fanout(from=a, n=2), got %+v", got)
	}
}

func TestAggregateInsideCycleIsRejected(t *testing.T) {
	db := setupTestDB(t)
	rules := mustParse(t, `
loop: {
	head: {rel: "loop", args: {id: "$X", n: "count($Y)"}}
	body: [{rel: "loop", args: {id: "$X", n: "$Y"}}]
}
`)
	if _, err := Evaluate(db, rules, "loop", nil, TemporalScope{}); err == nil || !strings.Contains(err.Error(), "recursive cycle") {
		t.Fatalf("want aggregate-in-cycle error, got %v", err)
	}
}

// A query must not evaluate rules it does not read. Before goal-directed
// evaluation, a zero-row answer re-ran every recursive rule, so an unrelated
// chain deeper than the iteration cap made an unrelated check fail.
func TestUnrelatedDeepRecursionDoesNotAffectQuery(t *testing.T) {
	db := setupTestDB(t)
	for i := 0; i < 120; i++ {
		addTestFact(t, db, "link", fmt.Sprintf(`{"from":"x%d","to":"x%d"}`, i, i+1))
	}
	addTestFact(t, db, "svc", `{"id":"s","status":"ok"}`)
	rules := mustParse(t, deepReachRules+`
failed_service: {
	head: {rel: "failed_service", args: {id: "$S"}}
	body: [{rel: "svc", args: {id: "$S", status: "failed"}}]
}
`)
	got, err := Evaluate(db, rules, "failed_service", nil, TemporalScope{})
	if err != nil {
		t.Fatalf("unrelated deep recursion broke the query: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want no failed services, got %+v", got)
	}

	if _, err := Evaluate(db, rules, "reach", nil, TemporalScope{}); err == nil || !strings.Contains(err.Error(), "not reached after 100") {
		t.Fatalf("a 120-step chain should exceed the default cap, got %v", err)
	}
	got, err = EvaluateWithOptions(db, rules, "reach", map[string]interface{}{"from": "x0"}, TemporalScope{}, EvalOptions{MaxIterations: 200})
	if err != nil {
		t.Fatalf("raised cap: %v", err)
	}
	if len(got) != 120 {
		t.Fatalf("want 120 reachable nodes from x0, got %d", len(got))
	}
}

func TestMutualRecursionAcrossRelations(t *testing.T) {
	db := setupTestDB(t)
	for i := 0; i < 6; i++ {
		addTestFact(t, db, "step", fmt.Sprintf(`{"from":"n%d","to":"n%d"}`, i, i+1))
	}
	addTestFact(t, db, "start", `{"node":"n0"}`)
	rules := mustParse(t, `
even_base: {
	head: {rel: "even", args: {node: "$N"}}
	body: [{rel: "start", args: {node: "$N"}}]
}
even_rec: {
	head: {rel: "even", args: {node: "$M"}}
	body: [{rel: "odd", args: {node: "$N"}}, {rel: "step", args: {from: "$N", to: "$M"}}]
}
odd_rec: {
	head: {rel: "odd", args: {node: "$M"}}
	body: [{rel: "even", args: {node: "$N"}}, {rel: "step", args: {from: "$N", to: "$M"}}]
}
`)
	got, err := Evaluate(db, rules, "odd", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if tupleSet(got) != `3 {"node":"n1"} {"node":"n3"} {"node":"n5"}` {
		t.Fatalf("want odd = n1, n3, n5, got %s", tupleSet(got))
	}
}

const deepReachRules = `
reach_base: {
	head: {rel: "reach", args: {from: "$A", to: "$B"}}
	body: [{rel: "link", args: {from: "$A", to: "$B"}}]
}
reach_rec: {
	head: {rel: "reach", args: {from: "$A", to: "$C"}}
	body: [{rel: "link", args: {from: "$A", to: "$B"}}, {rel: "reach", args: {from: "$B", to: "$C"}}]
}
`
