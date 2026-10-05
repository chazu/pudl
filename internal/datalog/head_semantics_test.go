package datalog

import (
	"strings"
	"testing"
)

const constantHeadRules = `
flagged: {
	head: {rel: "flagged", args: {id: "$X", severity: "high"}}
	body: [{rel: "svc", args: {id: "$X"}}]
}
`

func TestConstantHeadArgumentIsReturnedAndFilterable(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "svc", `{"id":"a"}`)
	rules := mustParse(t, constantHeadRules)

	got, err := Evaluate(db, rules, "flagged", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Args["severity"] != "high" || got[0].Args["id"] != "a" {
		t.Fatalf("want flagged(id=a, severity=high), got %+v", got)
	}

	got, err = Evaluate(db, rules, "flagged", map[string]interface{}{"severity": "high"}, TemporalScope{})
	if err != nil {
		t.Fatalf("filter on constant head key: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 tuple for severity=high, got %d", len(got))
	}
	got, err = Evaluate(db, rules, "flagged", map[string]interface{}{"severity": "low"}, TemporalScope{})
	if err != nil || len(got) != 0 {
		t.Fatalf("want 0 tuples for severity=low, got %d (err %v)", len(got), err)
	}
}

func TestConstantHeadArgumentInRecursiveRule(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "edge", `{"from":"a","to":"b"}`)
	addTestFact(t, db, "edge", `{"from":"b","to":"c"}`)
	rules := mustParse(t, `
reach_base: {
	head: {rel: "reach", args: {from: "$A", to: "$B", kind: "path"}}
	body: [{rel: "edge", args: {from: "$A", to: "$B"}}]
}
reach_rec: {
	head: {rel: "reach", args: {from: "$A", to: "$C", kind: "path"}}
	body: [{rel: "edge", args: {from: "$A", to: "$B"}}, {rel: "reach", args: {from: "$B", to: "$C", kind: "path"}}]
}
`)
	got, err := Evaluate(db, rules, "reach", map[string]interface{}{"kind": "path"}, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 reach tuples, got %+v", got)
	}
	for _, tup := range got {
		if tup.Args["kind"] != "path" {
			t.Fatalf("constant head key missing from %+v", tup)
		}
	}
}

func TestTwoRulesDerivingSameTupleReturnItOnce(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "svc", `{"id":"a"}`)
	addTestFact(t, db, "svc2", `{"id":"a"}`)
	rules := mustParse(t, `
either_a: {
	head: {rel: "either", args: {id: "$X"}}
	body: [{rel: "svc", args: {id: "$X"}}]
}
either_b: {
	head: {rel: "either", args: {id: "$X"}}
	body: [{rel: "svc2", args: {id: "$X"}}]
}
`)
	got, err := Evaluate(db, rules, "either", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 tuple, got %d: %+v", len(got), got)
	}
}

func TestRulesOfOneRelationMustShareHeadKeys(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "svc", `{"id":"a","name":"n"}`)
	rules := mustParse(t, `
one: {
	head: {rel: "out", args: {id: "$X"}}
	body: [{rel: "svc", args: {id: "$X"}}]
}
two: {
	head: {rel: "out", args: {name: "$N"}}
	body: [{rel: "svc", args: {name: "$N"}}]
}
`)
	if _, err := Evaluate(db, rules, "out", nil, TemporalScope{}); err == nil || !strings.Contains(err.Error(), "same keys") {
		t.Fatalf("want head-key mismatch error, got %v", err)
	}
	if problems := RuleSetProblems(rules); len(problems) != 1 {
		t.Fatalf("want 1 rule-set problem, got %v", problems)
	}
}

func TestQuotedCUEKeysAreUnquotedAndLiteral(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "svc", `{"app.kubernetes.io/name":"api","a":{"b":"nested"}}`)
	rules := mustParse(t, `
q: {
	head: {rel: "labelled", args: {"app-name": "$X"}}
	body: [{rel: "svc", args: {"app.kubernetes.io/name": "$X"}}]
}
`)
	got, err := Evaluate(db, rules, "labelled", map[string]interface{}{"app-name": "api"}, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Args["app-name"] != "api" {
		t.Fatalf("want labelled(app-name=api) with an unquoted key, got %+v", got)
	}
}

func TestArgumentKeyWithDoubleQuoteIsRejected(t *testing.T) {
	rules, err := ParseRulesFromSource(`
q: {
	head: {rel: "quoted", args: {"we\"ird": "$X"}}
	body: [{rel: "svc", args: {id: "$X"}}]
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if rules[0].Valid() || !strings.Contains(rules[0].LoadErr.Error(), "double quote") {
		t.Fatalf("want double-quote key rejected, got %v", rules[0].LoadErr)
	}
}

func mustParse(t testing.TB, src string) []Rule {
	t.Helper()
	rules, err := ParseRulesFromSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if problems := InvalidRules(rules); len(problems) > 0 {
		t.Fatalf("rules did not load: %v", problems[0].LoadErr)
	}
	return rules
}
