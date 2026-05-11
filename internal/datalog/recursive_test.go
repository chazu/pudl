package datalog

import (
	"testing"
)

func TestRecursiveTransitiveClosure(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "parent", `{"a":"alice","d":"bob"}`)
	addTestFact(t, db, "parent", `{"a":"bob","d":"charlie"}`)
	addTestFact(t, db, "parent", `{"a":"charlie","d":"dave"}`)

	rules := []Rule{
		{
			Name: "ancestor_base",
			Head: Atom{Rel: "ancestor", Args: map[string]Term{
				"a": Var("A"),
				"d": Var("D"),
			}},
			Body: []Atom{
				{Rel: "parent", Args: map[string]Term{"a": Var("A"), "d": Var("D")}},
			},
		},
		{
			Name: "ancestor_rec",
			Head: Atom{Rel: "ancestor", Args: map[string]Term{
				"a": Var("A"),
				"d": Var("D"),
			}},
			Body: []Atom{
				{Rel: "parent", Args: map[string]Term{"a": Var("A"), "d": Var("P")}},
				{Rel: "ancestor", Args: map[string]Term{"a": Var("P"), "d": Var("D")}},
			},
		},
	}

	results, err := EvalRecursive(db, rules, "ancestor", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	// Expected: alice->bob, alice->charlie, alice->dave, bob->charlie, bob->dave, charlie->dave
	if len(results) != 6 {
		t.Errorf("expected 6 ancestor pairs, got %d", len(results))
		for _, r := range results {
			t.Logf("  %v -> %v", r.Args["a"], r.Args["d"])
		}
	}

	found := make(map[string]bool)
	for _, r := range results {
		key := r.Args["a"].(string) + "->" + r.Args["d"].(string)
		found[key] = true
	}

	expected := []string{
		"alice->bob", "alice->charlie", "alice->dave",
		"bob->charlie", "bob->dave", "charlie->dave",
	}
	for _, e := range expected {
		if !found[e] {
			t.Errorf("missing expected pair: %s", e)
		}
	}
}

func TestRecursiveReachability(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "depends", `{"from":"api","to":"auth"}`)
	addTestFact(t, db, "depends", `{"from":"auth","to":"db"}`)
	addTestFact(t, db, "depends", `{"from":"db","to":"storage"}`)

	rules := []Rule{
		{
			Name: "reachable_base",
			Head: Atom{Rel: "reachable", Args: map[string]Term{
				"from": Var("A"),
				"to":   Var("B"),
			}},
			Body: []Atom{
				{Rel: "depends", Args: map[string]Term{"from": Var("A"), "to": Var("B")}},
			},
		},
		{
			Name: "reachable_rec",
			Head: Atom{Rel: "reachable", Args: map[string]Term{
				"from": Var("A"),
				"to":   Var("C"),
			}},
			Body: []Atom{
				{Rel: "depends", Args: map[string]Term{"from": Var("A"), "to": Var("B")}},
				{Rel: "reachable", Args: map[string]Term{"from": Var("B"), "to": Var("C")}},
			},
		},
	}

	results, err := EvalRecursive(db, rules, "reachable", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	// api->auth, api->db, api->storage, auth->db, auth->storage, db->storage
	if len(results) != 6 {
		t.Errorf("expected 6 reachable pairs, got %d", len(results))
		for _, r := range results {
			t.Logf("  %v -> %v", r.Args["from"], r.Args["to"])
		}
	}
}

func TestRecursiveConstraintFiltering(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "parent", `{"a":"alice","d":"bob"}`)
	addTestFact(t, db, "parent", `{"a":"bob","d":"charlie"}`)
	addTestFact(t, db, "parent", `{"a":"x","d":"y"}`)

	rules := []Rule{
		{
			Name: "ancestor_base",
			Head: Atom{Rel: "ancestor", Args: map[string]Term{
				"a": Var("A"),
				"d": Var("D"),
			}},
			Body: []Atom{
				{Rel: "parent", Args: map[string]Term{"a": Var("A"), "d": Var("D")}},
			},
		},
		{
			Name: "ancestor_rec",
			Head: Atom{Rel: "ancestor", Args: map[string]Term{
				"a": Var("A"),
				"d": Var("D"),
			}},
			Body: []Atom{
				{Rel: "parent", Args: map[string]Term{"a": Var("A"), "d": Var("P")}},
				{Rel: "ancestor", Args: map[string]Term{"a": Var("P"), "d": Var("D")}},
			},
		},
	}

	results, err := EvalRecursive(db, rules, "ancestor", map[string]interface{}{"a": "alice"}, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 ancestors of alice, got %d", len(results))
		for _, r := range results {
			t.Logf("  %v -> %v", r.Args["a"], r.Args["d"])
		}
	}
}

func TestRecursiveFixpointTermination(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "link", `{"from":"a","to":"b"}`)
	addTestFact(t, db, "link", `{"from":"b","to":"c"}`)

	rules := []Rule{
		{
			Name: "path_base",
			Head: Atom{Rel: "path", Args: map[string]Term{
				"from": Var("A"),
				"to":   Var("B"),
			}},
			Body: []Atom{
				{Rel: "link", Args: map[string]Term{"from": Var("A"), "to": Var("B")}},
			},
		},
		{
			Name: "path_rec",
			Head: Atom{Rel: "path", Args: map[string]Term{
				"from": Var("A"),
				"to":   Var("C"),
			}},
			Body: []Atom{
				{Rel: "link", Args: map[string]Term{"from": Var("A"), "to": Var("B")}},
				{Rel: "path", Args: map[string]Term{"from": Var("B"), "to": Var("C")}},
			},
		},
	}

	results, err := EvalRecursive(db, rules, "path", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	// a->b, a->c, b->c
	if len(results) != 3 {
		t.Errorf("expected 3 paths, got %d", len(results))
	}
}

func TestRecursiveEmptyBaseCase(t *testing.T) {
	db := setupTestDB(t)

	rules := []Rule{
		{
			Name: "ancestor_base",
			Head: Atom{Rel: "ancestor", Args: map[string]Term{
				"a": Var("A"),
				"d": Var("D"),
			}},
			Body: []Atom{
				{Rel: "parent", Args: map[string]Term{"a": Var("A"), "d": Var("D")}},
			},
		},
		{
			Name: "ancestor_rec",
			Head: Atom{Rel: "ancestor", Args: map[string]Term{
				"a": Var("A"),
				"d": Var("D"),
			}},
			Body: []Atom{
				{Rel: "parent", Args: map[string]Term{"a": Var("A"), "d": Var("P")}},
				{Rel: "ancestor", Args: map[string]Term{"a": Var("P"), "d": Var("D")}},
			},
		},
	}

	results, err := EvalRecursive(db, rules, "ancestor", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results with no base facts, got %d", len(results))
	}
}
