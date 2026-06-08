package factstore_test

import (
	"os"
	"testing"

	"pudl/pkg/eval"
	"pudl/pkg/factstore"
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
