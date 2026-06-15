package datalog

import (
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
)

// halfLife mirrors database.halfLifeSeconds (90 days) for assertions here.
const halfLife = 90 * 24 * 60 * 60

func TestFactScoredDecay(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().Unix()

	recent, err := db.AddFact(database.Fact{
		Relation: "playbook", Args: `{"worth":1.0,"bullet":"recent"}`,
		ValidStart: now, TxStart: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	old, err := db.AddFact(database.Fact{
		Relation: "playbook", Args: `{"worth":1.0,"bullet":"old"}`,
		ValidStart: now - halfLife, TxStart: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	rules, err := ParseRulesFromSource(`
scored: {
  head: {rel: "scored", args: {id: "$I", w: "$W"}}
  body: [{rel: "fact_scored", args: {id: "$I", relation: "playbook", decayed_worth: "$W"}}]
}`)
	if err != nil {
		t.Fatal(err)
	}

	results, err := Evaluate(db, rules, "scored", nil, TemporalScope{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	byID := map[string]float64{}
	for _, r := range results {
		id, _ := r.Args["id"].(string)
		w, _ := r.Args["w"].(float64)
		byID[id] = w
	}

	rw, ok := byID[recent.ID]
	if !ok {
		t.Fatalf("recent fact missing from scored results: %v", byID)
	}
	ow, ok := byID[old.ID]
	if !ok {
		t.Fatalf("old fact missing from scored results: %v", byID)
	}

	if rw < 0.99 {
		t.Errorf("recent decayed_worth = %v, want ~1.0", rw)
	}
	if ow < 0.49 || ow > 0.51 {
		t.Errorf("one-half-life-old decayed_worth = %v, want ~0.5", ow)
	}
	if rw <= ow {
		t.Errorf("recent (%v) should outrank old (%v)", rw, ow)
	}
}

// TestFactScoredJoinOnly verifies fact_scored cannot be queried directly (it is a
// join-only built-in, like catalog_entry).
func TestFactScoredJoinOnly(t *testing.T) {
	db := setupTestDB(t)
	_, err := Evaluate(db, nil, "fact_scored", nil, TemporalScope{})
	if err == nil {
		t.Fatal("expected error querying join-only fact_scored directly, got nil")
	}
}

// TestFactScoredThreshold verifies a rule can gate on decayed_worth with a
// comparison constraint as a recency-weighted recall filter.
func TestFactScoredThreshold(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().Unix()

	// Fresh fact (decayed ~1.0) and a very old fact (~3 half-lives, decayed ~0.125).
	fresh, _ := db.AddFact(database.Fact{Relation: "playbook", Args: `{"worth":1.0,"bullet":"fresh"}`, ValidStart: now, TxStart: now})
	stale, _ := db.AddFact(database.Fact{Relation: "playbook", Args: `{"worth":1.0,"bullet":"stale"}`, ValidStart: now - 3*halfLife, TxStart: now})

	rules, _ := ParseRulesFromSource(`
live: {
  head: {rel: "live", args: {id: "$I"}}
  body: [{rel: "fact_scored", args: {id: "$I", relation: "playbook", decayed_worth: ">0.25"}}]
}`)
	results, err := Evaluate(db, rules, "live", nil, TemporalScope{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected only the fresh fact above threshold, got %d: %v", len(results), results)
	}
	id, _ := results[0].Args["id"].(string)
	if id != fresh.ID {
		t.Errorf("expected fresh fact %s above threshold, got %s", fresh.ID[:8], id)
	}
	if id == stale.ID {
		t.Errorf("stale fact should be filtered out by decayed_worth > 0.25")
	}
}
