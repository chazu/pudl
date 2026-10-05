package datalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRulesKeepsMalformedRuleWithError(t *testing.T) {
	rules, err := ParseRulesFromSource(`
version: "1"
helper: {x: 1}
good: {
	head: {rel: "ok", args: {id: "$X"}}
	body: [{rel: "svc", args: {id: "$X"}}]
}
broken: {
	head: {rel: "broken", args: {id: "$X"}}
	body: [{args: {id: "$X"}}]
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("want 2 rules (non-rule fields ignored), got %d", len(rules))
	}
	invalid := InvalidRules(rules)
	if len(invalid) != 1 || invalid[0].Name != "broken" || invalid[0].Head.Rel != "broken" {
		t.Fatalf("want one invalid rule named broken with head rel, got %+v", invalid)
	}
	if !strings.Contains(invalid[0].LoadErr.Error(), "missing rel") {
		t.Fatalf("error should name the missing rel, got %v", invalid[0].LoadErr)
	}
}

func TestParseRulesRejectsUnboundHeadVariable(t *testing.T) {
	rules, err := ParseRulesFromSource(`
r: {
	head: {rel: "out", args: {id: "$X", other: "$Y"}}
	body: [{rel: "svc", args: {id: "$X"}}]
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if rules[0].Valid() || !strings.Contains(rules[0].LoadErr.Error(), "$Y") {
		t.Fatalf("want range-restriction error naming $Y, got %v", rules[0].LoadErr)
	}
}

func TestLoadRulesFromPathsReportsSourcePosition(t *testing.T) {
	dir := t.TempDir()
	src := "package rules\n\nbroken: {\n\thead: {rel: \"b\", args: {id: \"$X\"}}\n\tbody: [{args: {id: \"$X\"}}]\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "bad.cue"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, err := LoadRulesFromPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Valid() {
		t.Fatalf("want one invalid rule, got %+v", rules)
	}
	if !strings.Contains(rules[0].Source, "bad.cue:3") {
		t.Fatalf("want source position in bad.cue line 3, got %q", rules[0].Source)
	}
}

func TestEvaluateFailsWhenClosureContainsInvalidRule(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "svc", `{"id":"a"}`)
	rules, err := ParseRulesFromSource(`
top: {
	head: {rel: "top", args: {id: "$X"}}
	body: [{rel: "mid", args: {id: "$X"}}]
}
mid: {
	head: {rel: "mid", args: {id: "$X"}}
	body: [{args: {id: "$X"}}]
}
unrelated: {
	head: {rel: "unrelated", args: {id: "$X"}}
	body: [{rel: "svc", args: {id: "$X"}}]
}
`)
	if err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"mid", "top"} {
		if _, err := Evaluate(db, rules, rel, nil, TemporalScope{}); err == nil || !strings.Contains(err.Error(), "invalid rule") {
			t.Fatalf("query %s: want invalid-rule error, got %v", rel, err)
		}
	}

	got, err := Evaluate(db, rules, "unrelated", nil, TemporalScope{})
	if err != nil {
		t.Fatalf("unrelated relation should still evaluate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 unrelated tuple, got %d", len(got))
	}
}

func TestEvaluateFailsWhenInvalidRuleHeadIsUnknown(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "svc", `{"id":"a"}`)
	rules, err := ParseRulesFromSource(`
mystery: {
	head: {args: {id: "$X"}}
	body: [{rel: "svc", args: {id: "$X"}}]
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Evaluate(db, rules, "svc", nil, TemporalScope{}); err == nil {
		t.Fatal("an invalid rule with an unknown head must block every query")
	}
}
