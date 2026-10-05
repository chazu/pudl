package cmd

import (
	"os"
	"path/filepath"
	"strings"
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
	if !rules[0].Valid() {
		t.Errorf("scaffold must be a valid rule: %v", rules[0].LoadErr)
	}
}

func TestRuleAddRejectsInvalidRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.cue")
	src := "broken: {\n\thead: {rel: \"b\", args: {id: \"$X\"}}\n\tbody: [{args: {id: \"$X\"}}]\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ruleAddCmd.RunE(ruleAddCmd, []string{path})
	if err == nil || !strings.Contains(err.Error(), "invalid rule") || !strings.Contains(err.Error(), "missing rel") {
		t.Fatalf("want invalid-rule validation error, got %v", err)
	}
}
