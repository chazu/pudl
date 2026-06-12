package factstore_test

import (
	"errors"
	"os"
	"testing"

	"github.com/chazu/pudl/pkg/eval"
	"github.com/chazu/pudl/pkg/factstore"
)

func openStore(t *testing.T) *factstore.Store {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "pudl-factstore-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	s, err := factstore.Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func addFact(t *testing.T, s *factstore.Store, relation, args string) {
	t.Helper()
	if _, err := s.AddFact(factstore.Fact{
		Relation:   relation,
		Args:       args,
		ValidStart: 1,
		TxStart:    1,
	}); err != nil {
		t.Fatal(err)
	}
}

// Query a base relation with no rules — exercises the EDB fallback path.
func TestQueryBaseRelation(t *testing.T) {
	s := openStore(t)
	addFact(t, s, "depends", `{"from":"api","to":"svc-a"}`)
	addFact(t, s, "depends", `{"from":"api","to":"svc-b"}`)

	results, err := s.Query(factstore.QueryOptions{Relation: "depends"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 tuples, got %d", len(results))
	}
}

// Query with a constraint filters fallback results.
func TestQueryBaseRelationConstrained(t *testing.T) {
	s := openStore(t)
	addFact(t, s, "depends", `{"from":"api","to":"svc-a"}`)
	addFact(t, s, "depends", `{"from":"web","to":"svc-b"}`)

	results, err := s.Query(factstore.QueryOptions{
		Relation:    "depends",
		Constraints: map[string]interface{}{"from": "api"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(results))
	}
}

// Query a derived relation via a non-recursive rule — exercises the SQL path.
func TestQueryDerivedNonRecursive(t *testing.T) {
	s := openStore(t)
	addFact(t, s, "depends", `{"from":"api","to":"svc-a"}`)
	addFact(t, s, "observation", `{"target":"svc-a","kind":"unhealthy"}`)

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

	results, err := s.Query(factstore.QueryOptions{Relation: "at_risk", Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 derived tuple, got %d", len(results))
	}
	if got := results[0].Args["service"]; got != "api" {
		t.Fatalf("expected service=api, got %v", got)
	}
}

// Query a recursive relation — exercises the recursive fixpoint fallback.
func TestQueryDerivedRecursive(t *testing.T) {
	s := openStore(t)
	addFact(t, s, "edge", `{"from":"a","to":"b"}`)
	addFact(t, s, "edge", `{"from":"b","to":"c"}`)
	addFact(t, s, "edge", `{"from":"c","to":"d"}`)

	rules, err := eval.ParseRulesFromSource(`
reach_base: {
	head: {rel: "reach", args: {from: "$X", to: "$Y"}}
	body: [{rel: "edge", args: {from: "$X", to: "$Y"}}]
}
reach_step: {
	head: {rel: "reach", args: {from: "$X", to: "$Z"}}
	body: [
		{rel: "edge", args: {from: "$X", to: "$Y"}},
		{rel: "reach", args: {from: "$Y", to: "$Z"}},
	]
}
`)
	if err != nil {
		t.Fatal(err)
	}

	results, err := s.Query(factstore.QueryOptions{Relation: "reach", Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	// a→b,a→c,a→d, b→c,b→d, c→d = 6 reachable pairs.
	if len(results) != 6 {
		t.Fatalf("expected 6 reachable pairs, got %d", len(results))
	}
}

// Facts can be queried back through the bitemporal filter API.
func TestQueryFactsRoundTrip(t *testing.T) {
	s := openStore(t)
	addFact(t, s, "depends", `{"from":"api","to":"svc-a"}`)

	facts, err := s.QueryFacts(factstore.FactFilter{Relation: "depends"})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
}

// Transact gives check-then-write atomicity: reads and writes inside the
// callback are one transaction, and an error rolls every write back.
func TestTransactCommitAndRollback(t *testing.T) {
	s := openStore(t)

	err := s.Transact(func(tx *factstore.Tx) error {
		existing, err := tx.QueryFacts(factstore.FactFilter{Relation: "claim"})
		if err != nil {
			return err
		}
		if len(existing) != 0 {
			t.Fatalf("expected empty relation, got %d facts", len(existing))
		}
		_, err = tx.AddFact(factstore.Fact{Relation: "claim", Args: `{"owner":"a"}`})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	facts, err := s.QueryFacts(factstore.FactFilter{Relation: "claim"})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 committed fact, got %d", len(facts))
	}

	boom := errors.New("invariant violated")
	err = s.Transact(func(tx *factstore.Tx) error {
		if _, err := tx.AddFact(factstore.Fact{Relation: "claim", Args: `{"owner":"b"}`}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected callback error to propagate, got %v", err)
	}

	facts, err = s.QueryFacts(factstore.FactFilter{Relation: "claim"})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected rollback to keep 1 fact, got %d", len(facts))
	}
}
