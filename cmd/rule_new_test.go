package cmd

import (
	"testing"

	"github.com/chazu/pudl/internal/datalog"
)

func TestRuleScaffoldSourceLoads(t *testing.T) {
	rules, err := datalog.ParseRulesFromSource(ruleScaffoldSource("my-rule"))
	if err != nil {
		t.Fatalf("scaffold does not parse: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Name != "my-rule" {
		t.Errorf("rule name = %q, want %q", rules[0].Name, "my-rule")
	}
	if rules[0].Head.Rel != "derived_relation" || len(rules[0].Body) != 1 {
		t.Errorf("unexpected rule shape: %+v", rules[0])
	}
}
