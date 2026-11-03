package datalog

import (
	"strings"
	"testing"
)

const groundFactRules = `
edge_type_in_subnet: {
	head: {rel: "edge_type", args: {type: "in_subnet", category: "containment"}}
}
edge_type_runs_as: {
	head: {rel: "edge_type", args: {type: "runs_as", category: "iam"}}
	body: []
}
edge_type_derived: {
	head: {rel: "edge_type", args: {type: "$T", category: "other"}}
	body: [{rel: "seen_edge", args: {type: "$T"}}]
}
typed_edge: {
	head: {rel: "typed_edge", args: {type: "$T", category: "$C"}}
	body: [
		{rel: "seen_edge", args: {type: "$T"}},
		{rel: "edge_type", args: {type: "$T", category: "$C"}},
	]
}
`

func TestGroundFactsLoadAndEvaluate(t *testing.T) {
	rules, err := ParseRulesFromSource(groundFactRules)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if !r.Valid() {
			t.Fatalf("rule %s did not load: %v", r.Name, r.LoadErr)
		}
	}
	db := setupTestDB(t)
	addTestFact(t, db, "seen_edge", `{"type":"in_subnet"}`)

	got, err := Evaluate(db, rules, "edge_type", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"in_subnet/containment": true, "runs_as/iam": true, "in_subnet/other": true}
	if len(got) != len(want) {
		t.Fatalf("edge_type = %v, want %d rows", got, len(want))
	}
	for _, tu := range got {
		key := tu.Args["type"].(string) + "/" + tu.Args["category"].(string)
		if !want[key] {
			t.Errorf("unexpected edge_type row %s", key)
		}
	}

	// A rule joining stored facts with ground facts.
	joined, err := Evaluate(db, rules, "typed_edge", map[string]interface{}{"category": "containment"}, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(joined) != 1 || joined[0].Args["type"] != "in_subnet" {
		t.Fatalf("typed_edge = %v, want one in_subnet row", joined)
	}
}

func TestGroundFactsSeedRecursion(t *testing.T) {
	rules, err := ParseRulesFromSource(`
link_ab: {head: {rel: "link", args: {from: "a", to: "b"}}}
link_bc: {head: {rel: "link", args: {from: "b", to: "c"}}}
reach_base: {
	head: {rel: "reach", args: {from: "$X", to: "$Y"}}
	body: [{rel: "link", args: {from: "$X", to: "$Y"}}]
}
reach_step: {
	head: {rel: "reach", args: {from: "$X", to: "$Z"}}
	body: [
		{rel: "reach", args: {from: "$X", to: "$Y"}},
		{rel: "link", args: {from: "$Y", to: "$Z"}},
	]
}
`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Evaluate(setupTestDB(t), rules, "reach", map[string]interface{}{"from": "a"}, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("reach from a = %v, want b and c", got)
	}
}

func TestGroundFactRejectsVariableHead(t *testing.T) {
	rules, err := ParseRulesFromSource(`bad: {head: {rel: "edge_type", args: {type: "$T"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Valid() {
		t.Fatalf("want one invalid rule, got %+v", rules)
	}
	if !strings.Contains(rules[0].LoadErr.Error(), "constants only") {
		t.Errorf("error = %v, want it to say facts take constants only", rules[0].LoadErr)
	}
}
