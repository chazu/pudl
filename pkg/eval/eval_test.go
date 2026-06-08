package eval_test

import (
	"testing"

	"github.com/chazu/pudl/pkg/eval"
)

func TestParseRulesFromSource(t *testing.T) {
	rules, err := eval.ParseRulesFromSource(`
at_risk: {
	head: {rel: "at_risk", args: {service: "$S"}}
	body: [
		{rel: "depends", args: {from: "$S", to: "$D"}},
		{rel: "observation", args: {target: "$D", kind: "unhealthy"}},
	]
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if r.Head.Rel != "at_risk" {
		t.Errorf("expected head rel at_risk, got %s", r.Head.Rel)
	}
	if len(r.Body) != 2 {
		t.Errorf("expected 2 body atoms, got %d", len(r.Body))
	}
}

func TestVarVal(t *testing.T) {
	v := eval.Var("X")
	if !v.IsVariable() {
		t.Errorf("Var should produce a variable term")
	}
	g := eval.Val("api")
	if g.IsVariable() {
		t.Errorf("Val should produce a ground term")
	}
}
