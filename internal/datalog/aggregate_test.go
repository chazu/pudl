package datalog

import "testing"

func TestAggregateCountGroupBy(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "feedback", `{"target":"X","verdict":"harmful","source":"a"}`)
	addTestFact(t, db, "feedback", `{"target":"X","verdict":"harmful","source":"b"}`)
	addTestFact(t, db, "feedback", `{"target":"X","verdict":"harmful","source":"c"}`)
	addTestFact(t, db, "feedback", `{"target":"Y","verdict":"harmful","source":"a"}`)
	addTestFact(t, db, "feedback", `{"target":"X","verdict":"helpful","source":"d"}`)

	rules, err := ParseRulesFromSource(`
harmful_count: {
  head: {rel: "harmful_count", args: {target: "$T", n: "count($S)"}}
  body: [{rel: "feedback", args: {target: "$T", verdict: "harmful", source: "$S"}}]
}`)
	if err != nil {
		t.Fatal(err)
	}

	results, err := Evaluate(db, rules, "harmful_count", nil, TemporalScope{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	got := map[string]float64{}
	for _, r := range results {
		tgt, _ := r.Args["target"].(string)
		n, _ := r.Args["n"].(float64)
		got[tgt] = n
	}
	if got["X"] != 3 {
		t.Errorf("target X harmful count = %v, want 3", got["X"])
	}
	if got["Y"] != 1 {
		t.Errorf("target Y harmful count = %v, want 1", got["Y"])
	}
	if _, ok := got["X"]; len(got) != 2 || !ok {
		t.Errorf("expected 2 groups (X,Y), got %v", got)
	}
}

func TestAggregateConstraintFilter(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "feedback", `{"target":"X","verdict":"harmful","source":"a"}`)
	addTestFact(t, db, "feedback", `{"target":"X","verdict":"harmful","source":"b"}`)
	addTestFact(t, db, "feedback", `{"target":"Y","verdict":"harmful","source":"a"}`)

	rules, _ := ParseRulesFromSource(`
hc: {
  head: {rel: "hc", args: {target: "$T", n: "count($S)"}}
  body: [{rel: "feedback", args: {target: "$T", verdict: "harmful", source: "$S"}}]
}`)

	results, err := Evaluate(db, rules, "hc", map[string]interface{}{"target": "X"}, TemporalScope{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 row for target X, got %d: %v", len(results), results)
	}
	if n, _ := results[0].Args["n"].(float64); n != 2 {
		t.Errorf("target X count = %v, want 2", n)
	}
}

func TestAggregatePureNoGroupKey(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "feedback", `{"target":"X","verdict":"harmful","source":"a"}`)
	addTestFact(t, db, "feedback", `{"target":"Y","verdict":"helpful","source":"b"}`)
	addTestFact(t, db, "feedback", `{"target":"Z","verdict":"neutral","source":"c"}`)

	rules, _ := ParseRulesFromSource(`
total: {
  head: {rel: "total", args: {n: "count($T)"}}
  body: [{rel: "feedback", args: {target: "$T"}}]
}`)

	results, err := Evaluate(db, rules, "total", nil, TemporalScope{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("pure aggregate should yield 1 row, got %d", len(results))
	}
	if n, _ := results[0].Args["n"].(float64); n != 3 {
		t.Errorf("total count = %v, want 3", n)
	}
}

func TestAggregateInBodyRejected(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "feedback", `{"target":"X","verdict":"harmful","source":"a"}`)

	rules, _ := ParseRulesFromSource(`
bad: {
  head: {rel: "bad", args: {target: "$T"}}
  body: [{rel: "feedback", args: {target: "$T", source: "count($S)"}}]
}`)

	_, err := Evaluate(db, rules, "bad", nil, TemporalScope{})
	if err == nil {
		t.Fatal("expected error for aggregate in rule body, got nil")
	}
}

func TestAggregateRecursiveRejected(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "edge", `{"from":"a","to":"b"}`)

	// reach is recursive (body references reach) and also aggregates → rejected.
	rules, _ := ParseRulesFromSource(`
base: {
  head: {rel: "reach", args: {from: "$X", to: "$Y", n: "count($X)"}}
  body: [{rel: "edge", args: {from: "$X", to: "$Y"}}]
}
step: {
  head: {rel: "reach", args: {from: "$X", to: "$Z", n: "count($X)"}}
  body: [{rel: "edge", args: {from: "$X", to: "$Y"}}, {rel: "reach", args: {from: "$Y", to: "$Z"}}]
}`)

	_, err := Evaluate(db, rules, "reach", nil, TemporalScope{})
	if err == nil {
		t.Fatal("expected error for aggregation in recursive rule, got nil")
	}
}
