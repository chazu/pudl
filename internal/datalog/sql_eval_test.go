package datalog

import (
	"os"
	"testing"

	"github.com/chazu/pudl/internal/database"
)

func setupTestDB(t *testing.T) *database.CatalogDB {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "pudl-sql-eval-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	db, err := database.NewCatalogDB(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func addTestFact(t *testing.T, db *database.CatalogDB, relation, args string) {
	t.Helper()
	_, err := db.AddFact(database.Fact{
		Relation:   relation,
		Args:       args,
		ValidStart: 1,
		TxStart:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLEvalSimpleFactLookup(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "depends", `{"from":"api","to":"svc-a"}`)
	addTestFact(t, db, "depends", `{"from":"api","to":"svc-b"}`)

	eval := NewSQLEvaluator(db, nil, TemporalScope{})
	results, err := eval.Query("depends", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 facts, got %d", len(results))
	}
}

func TestSQLEvalSingleRuleDerivation(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "depends", `{"from":"api","to":"svc-a"}`)
	addTestFact(t, db, "observation", `{"target":"svc-a","kind":"unhealthy"}`)

	rules := []Rule{
		{
			Name: "at_risk",
			Head: Atom{Rel: "at_risk", Args: map[string]Term{
				"service": Var("S"),
			}},
			Body: []Atom{
				{Rel: "depends", Args: map[string]Term{
					"from": Var("S"),
					"to":   Var("D"),
				}},
				{Rel: "observation", Args: map[string]Term{
					"target": Var("D"),
					"kind":   Val("unhealthy"),
				}},
			},
		},
	}

	eval := NewSQLEvaluator(db, rules, TemporalScope{})
	results, err := eval.Query("at_risk", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Args["service"] != "api" {
		t.Errorf("expected service=api, got %v", results[0].Args["service"])
	}
}

func TestSQLEvalMultiJoin(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "edge", `{"from":"a","to":"b"}`)
	addTestFact(t, db, "edge", `{"from":"b","to":"c"}`)
	addTestFact(t, db, "node", `{"name":"b","active":true}`)

	rules := []Rule{
		{
			Name: "chain",
			Head: Atom{Rel: "chain", Args: map[string]Term{
				"start": Var("A"),
				"end":   Var("C"),
			}},
			Body: []Atom{
				{Rel: "edge", Args: map[string]Term{"from": Var("A"), "to": Var("B")}},
				{Rel: "edge", Args: map[string]Term{"from": Var("B"), "to": Var("C")}},
				{Rel: "node", Args: map[string]Term{"name": Var("B"), "active": Val(true)}},
			},
		},
	}

	eval := NewSQLEvaluator(db, rules, TemporalScope{})
	results, err := eval.Query("chain", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 chain result, got %d", len(results))
	}
	if results[0].Args["start"] != "a" || results[0].Args["end"] != "c" {
		t.Errorf("expected start=a, end=c, got %v", results[0].Args)
	}
}

func TestSQLEvalConstraintFiltering(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "depends", `{"from":"api","to":"svc-a"}`)
	addTestFact(t, db, "depends", `{"from":"web","to":"svc-b"}`)
	addTestFact(t, db, "observation", `{"target":"svc-a","kind":"unhealthy"}`)
	addTestFact(t, db, "observation", `{"target":"svc-b","kind":"unhealthy"}`)

	rules := []Rule{
		{
			Name: "at_risk",
			Head: Atom{Rel: "at_risk", Args: map[string]Term{
				"service": Var("S"),
			}},
			Body: []Atom{
				{Rel: "depends", Args: map[string]Term{
					"from": Var("S"),
					"to":   Var("D"),
				}},
				{Rel: "observation", Args: map[string]Term{
					"target": Var("D"),
					"kind":   Val("unhealthy"),
				}},
			},
		},
	}

	eval := NewSQLEvaluator(db, rules, TemporalScope{})
	results, err := eval.Query("at_risk", map[string]interface{}{"service": "api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(results))
	}
	if results[0].Args["service"] != "api" {
		t.Errorf("expected service=api, got %v", results[0].Args["service"])
	}
}

func TestSQLEvalFallbackEDB(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "depends", `{"from":"api","to":"svc-a"}`)

	rules := []Rule{
		{
			Name: "at_risk",
			Head: Atom{Rel: "at_risk", Args: map[string]Term{"s": Var("S")}},
			Body: []Atom{
				{Rel: "observation", Args: map[string]Term{"target": Var("S")}},
			},
		},
	}

	eval := NewSQLEvaluator(db, rules, TemporalScope{})
	results, err := eval.Query("depends", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 fact via EDB fallback, got %d", len(results))
	}
}

func TestSQLEvalTemporalMode(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.AddFact(database.Fact{
		Relation:   "obs",
		Args:       `{"x":"hello"}`,
		ValidStart: 500,
		TxStart:    500,
	})
	if err != nil {
		t.Fatal(err)
	}

	rules := []Rule{
		{
			Name: "derived",
			Head: Atom{Rel: "derived", Args: map[string]Term{"val": Var("X")}},
			Body: []Atom{
				{Rel: "obs", Args: map[string]Term{"x": Var("X")}},
			},
		},
	}

	validAt := int64(600)
	eval := NewSQLEvaluator(db, rules, TemporalScope{ValidAt: &validAt})
	results, err := eval.Query("derived", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 temporal result, got %d", len(results))
	}
	if results[0].Args["val"] != "hello" {
		t.Errorf("expected val=hello, got %v", results[0].Args["val"])
	}

	earlyValid := int64(100)
	eval2 := NewSQLEvaluator(db, rules, TemporalScope{ValidAt: &earlyValid})
	results2, err := eval2.Query("derived", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results2) != 0 {
		t.Errorf("expected 0 results before valid_start, got %d", len(results2))
	}
}
