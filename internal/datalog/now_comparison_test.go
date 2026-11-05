package datalog

import (
	"sort"
	"testing"
	"time"
)

// keysWhere evaluates `old(id) :- key(id, created: <cmp>)` and returns the ids.
func keysWhere(t *testing.T, cmp string, scope TemporalScope) []string {
	t.Helper()
	db := setupTestDB(t)
	day := 24 * time.Hour
	now := time.Now().UTC()
	addTestFact(t, db, "key", `{"id":"new","created":"`+now.Add(-10*day).Format(time.RFC3339)+`"}`)
	addTestFact(t, db, "key", `{"id":"old","created":"`+now.Add(-200*day).Format(time.RFC3339)+`"}`)
	addTestFact(t, db, "key", `{"id":"ancient","created":"2001-02-03T04:05:06.789Z"}`)
	addTestFact(t, db, "key", `{"id":"nodate","created":"9999-12-31 is not a date?"}`)
	addTestFact(t, db, "key", `{"id":"number","created":42}`)
	rules, err := ParseRulesFromSource(`
old: {
  head: {rel: "old", args: {id: "$I"}}
  body: [{rel: "key", args: {id: "$I", created: "` + cmp + `"}}]
}`)
	if err != nil {
		t.Fatal(err)
	}
	results, err := Evaluate(db, rules, "old", nil, scope)
	if err != nil {
		t.Fatalf("evaluate %s: %v", cmp, err)
	}
	var ids []string
	for _, r := range results {
		id, _ := r.Args["id"].(string)
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func TestNowComparisonAgainstTheCurrentTime(t *testing.T) {
	cases := map[string][]string{
		"<now-90d":     {"ancient", "old"},
		"<= now - 90d": {"ancient", "old"},
		">now-90d":     {"new"},
		">=now-5000h":  {"new", "old"},
		"<now":         {"ancient", "new", "old"},
		">now":         nil,
	}
	for cmp, want := range cases {
		if got := keysWhere(t, cmp, TemporalScope{}); !equalStrings(got, want) {
			t.Errorf("%s: got %v, want %v (non-dates never match)", cmp, got, want)
		}
	}
}

func TestNowComparisonFollowsTimeTravel(t *testing.T) {
	// As of 150 days ago, "now-90d" is 240 days ago: the 200-day-old key was
	// newer than that.
	asOf := time.Now().Add(-150 * 24 * time.Hour).Unix()
	if got := keysWhere(t, ">now-90d", TemporalScope{ValidAt: &asOf}); !equalStrings(got, []string{"new", "old"}) {
		t.Errorf(">now-90d as of 150 days ago: got %v", got)
	}
	if got := keysWhere(t, "<now-90d", TemporalScope{ValidAt: &asOf}); !equalStrings(got, []string{"ancient"}) {
		t.Errorf("<now-90d as of 150 days ago: got %v", got)
	}
}

func TestNowTermParsing(t *testing.T) {
	for in, ago := range map[string]int64{"<now": 0, "<now-1m": 60, ">= now-2h": 7200, "<now-90d": 90 * 86400} {
		term := ParseTerm(in)
		if !term.IsComparison() || term.Ago == nil || *term.Ago != ago {
			t.Errorf("%q: got %+v, want a now comparison %d seconds ago", in, term, ago)
		}
	}
	for _, in := range []string{"!=now", "<now-90", "<now-3w", "<nowish", "now"} {
		if term := ParseTerm(in); term.Ago != nil {
			t.Errorf("%q should not be a now comparison: %+v", in, term)
		}
	}
	rules, err := ParseRulesFromSource(`r: {head: {rel: "r", args: {a: "<now-1d"}}, body: [{rel: "x", args: {a: "$A"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRule(rules[0]); err == nil {
		t.Error("a now comparison in a head must be rejected, like a numeric one")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
