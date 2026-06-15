package datalog

import "testing"

func TestComparisonOperators(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "metric", `{"host":"a","load":90}`)
	addTestFact(t, db, "metric", `{"host":"b","load":50}`)
	addTestFact(t, db, "metric", `{"host":"c","load":80}`)

	cases := []struct {
		name string
		op   string
		want map[string]bool // hosts expected
	}{
		{"gt", ">80", map[string]bool{"a": true}},
		{"gte", ">=80", map[string]bool{"a": true, "c": true}},
		{"lt", "<80", map[string]bool{"b": true}},
		{"lte", "<=80", map[string]bool{"b": true, "c": true}},
		{"ne", "!=80", map[string]bool{"a": true, "b": true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := `
hot: {
  head: {rel: "hot", args: {host: "$H"}}
  body: [{rel: "metric", args: {host: "$H", load: "` + tc.op + `"}}]
}`
			rules, err := ParseRulesFromSource(src)
			if err != nil {
				t.Fatal(err)
			}
			results, err := Evaluate(db, rules, "hot", nil, TemporalScope{})
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			got := map[string]bool{}
			for _, r := range results {
				h, _ := r.Args["host"].(string)
				got[h] = true
			}
			if len(got) != len(tc.want) {
				t.Fatalf("op %s: got hosts %v, want %v", tc.op, got, tc.want)
			}
			for h := range tc.want {
				if !got[h] {
					t.Errorf("op %s: missing host %s (got %v)", tc.op, h, got)
				}
			}
		})
	}
}

func TestComparisonFloatBound(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "obs", `{"id":"x","worth":0.9}`)
	addTestFact(t, db, "obs", `{"id":"y","worth":0.2}`)

	rules, _ := ParseRulesFromSource(`
strong: {
  head: {rel: "strong", args: {id: "$I"}}
  body: [{rel: "obs", args: {id: "$I", worth: ">0.5"}}]
}`)
	results, err := Evaluate(db, rules, "strong", nil, TemporalScope{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d: %v", len(results), results)
	}
	if id, _ := results[0].Args["id"].(string); id != "x" {
		t.Errorf("want id x, got %s", id)
	}
}
