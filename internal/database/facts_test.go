package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddFact(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f := Fact{
		Relation: "observation",
		Args:     `{"kind":"obstacle","description":"circular dep in auth"}`,
		Source:   "agent-1",
	}

	got, err := db.AddFact(f)
	require.NoError(t, err)
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, "observation", got.Relation)
	assert.NotZero(t, got.ValidStart)
	assert.NotZero(t, got.TxStart)
}

func TestAddFactValidation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := db.AddFact(Fact{Args: `{}`})
	assert.Error(t, err, "empty relation should fail")

	_, err = db.AddFact(Fact{Relation: "test"})
	assert.Error(t, err, "empty args should fail")
}

func TestAddFactDedup(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().Unix()
	f := Fact{
		Relation:   "observation",
		Args:       `{"kind":"obstacle"}`,
		ValidStart: now,
		Source:     "agent-1",
	}

	got1, err := db.AddFact(f)
	require.NoError(t, err)

	// Same content produces same ID — insert fails (PK conflict)
	_, err = db.AddFact(f)
	assert.Error(t, err, "duplicate fact should fail on PK")

	// Different source → different ID
	f.Source = "agent-2"
	got2, err := db.AddFact(f)
	require.NoError(t, err)
	assert.NotEqual(t, got1.ID, got2.ID)
}

func TestGetFact(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f, err := db.AddFact(Fact{
		Relation: "depends",
		Args:     `{"from":"api","to":"db"}`,
		Source:   "scanner",
	})
	require.NoError(t, err)

	got, err := db.GetFact(f.ID)
	require.NoError(t, err)
	assert.Equal(t, f.ID, got.ID)
	assert.Equal(t, "depends", got.Relation)
	assert.Equal(t, "scanner", got.Source)

	_, err = db.GetFact("nonexistent")
	assert.Error(t, err)
}

func TestRetractFact(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"kind":"pattern"}`,
		Source:   "agent-1",
	})
	require.NoError(t, err)

	err = db.RetractFact(f.ID)
	require.NoError(t, err)

	// Fact still exists but has tx_end set
	got, err := db.GetFact(f.ID)
	require.NoError(t, err)
	assert.NotNil(t, got.TxEnd)

	// Double retraction fails
	err = db.RetractFact(f.ID)
	assert.Error(t, err)
}

func TestInvalidateFact(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"kind":"pattern"}`,
		Source:   "agent-1",
	})
	require.NoError(t, err)

	err = db.InvalidateFact(f.ID)
	require.NoError(t, err)

	got, err := db.GetFact(f.ID)
	require.NoError(t, err)
	assert.NotNil(t, got.ValidEnd)
	assert.Nil(t, got.TxEnd) // not retracted, just no longer valid

	// Double invalidation fails
	err = db.InvalidateFact(f.ID)
	assert.Error(t, err)
}

func TestQueryFactsAsOfNow(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Add two current facts, retract one
	f1, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"kind":"pattern","id":"1"}`,
		Source:   "agent-1",
	})
	require.NoError(t, err)

	_, err = db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"kind":"obstacle","id":"2"}`,
		Source:   "agent-1",
	})
	require.NoError(t, err)

	err = db.RetractFact(f1.ID)
	require.NoError(t, err)

	// AsOfNow should return only the non-retracted fact
	facts, err := db.QueryFacts(FactFilter{Relation: "observation"})
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Contains(t, facts[0].Args, "obstacle")
}

func TestQueryFactsAsOfValid(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	past := time.Now().Add(-1 * time.Hour).Unix()
	now := time.Now().Unix()

	// Fact valid in the past, invalidated before now
	invalidated := now - 1800 // 30 min ago
	_, err := db.AddFact(Fact{
		Relation:   "state",
		Args:       `{"service":"api","status":"healthy"}`,
		ValidStart: past,
		TxStart:    past,
		Source:     "monitor",
	})
	require.NoError(t, err)

	// Fact valid starting 30 min ago
	_, err = db.AddFact(Fact{
		Relation:   "state",
		Args:       `{"service":"api","status":"degraded"}`,
		ValidStart: invalidated,
		TxStart:    invalidated,
		Source:     "monitor",
	})
	require.NoError(t, err)

	// Query at a point between past and invalidated
	queryTime := past + 900 // 45 min ago
	facts, err := db.QueryFacts(FactFilter{
		Relation: "state",
		ValidAt:  &queryTime,
	})
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Contains(t, facts[0].Args, "healthy")

	// Query at now: both visible (neither has valid_end set)
	facts, err = db.QueryFacts(FactFilter{
		Relation: "state",
		ValidAt:  &now,
	})
	require.NoError(t, err)
	assert.Len(t, facts, 2)
}

func TestQueryFactsAsOfTransaction(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	t1 := time.Now().Add(-2 * time.Hour).Unix()
	t2 := time.Now().Add(-1 * time.Hour).Unix()

	// Fact asserted at t1
	_, err := db.AddFact(Fact{
		Relation:   "observation",
		Args:       `{"kind":"early"}`,
		ValidStart: t1,
		TxStart:    t1,
		Source:     "agent-1",
	})
	require.NoError(t, err)

	// Fact asserted at t2
	_, err = db.AddFact(Fact{
		Relation:   "observation",
		Args:       `{"kind":"late"}`,
		ValidStart: t2,
		TxStart:    t2,
		Source:     "agent-1",
	})
	require.NoError(t, err)

	// AsOfTransaction(t1 + 1): only the first fact was known
	queryTx := t1 + 1
	facts, err := db.QueryFacts(FactFilter{
		Relation: "observation",
		TxAt:     &queryTx,
	})
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Contains(t, facts[0].Args, "early")
}

func TestQueryFactsAsOf(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	t1 := time.Now().Add(-3 * time.Hour).Unix()
	t2 := time.Now().Add(-2 * time.Hour).Unix()
	t3 := time.Now().Add(-1 * time.Hour).Unix()

	// Fact: valid from t1, asserted at t2
	_, err := db.AddFact(Fact{
		Relation:   "config",
		Args:       `{"key":"timeout","value":"30s"}`,
		ValidStart: t1,
		TxStart:    t2,
		Source:     "discovery",
	})
	require.NoError(t, err)

	// AsOf(validT=t1, txT=t1): not yet known at t1
	facts, err := db.QueryFacts(FactFilter{
		Relation: "config",
		ValidAt:  &t1,
		TxAt:     &t1,
	})
	require.NoError(t, err)
	assert.Len(t, facts, 0)

	// AsOf(validT=t1, txT=t3): known at t3 that it was true at t1
	facts, err = db.QueryFacts(FactFilter{
		Relation: "config",
		ValidAt:  &t1,
		TxAt:     &t3,
	})
	require.NoError(t, err)
	assert.Len(t, facts, 1)
}

func TestQueryFactsRequiresRelation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := db.QueryFacts(FactFilter{})
	assert.Error(t, err)
}

func TestComputeFactID(t *testing.T) {
	id1 := ComputeFactID("obs", `{"a":1}`, 1000, "agent-1")
	id2 := ComputeFactID("obs", `{"a":1}`, 1000, "agent-1")
	id3 := ComputeFactID("obs", `{"a":1}`, 1000, "agent-2")

	assert.Equal(t, id1, id2, "same inputs produce same ID")
	assert.NotEqual(t, id1, id3, "different source produces different ID")
	assert.Len(t, id1, 64, "SHA256 hex is 64 chars")
}

func TestCanonicalizeJSON(t *testing.T) {
	// Different key ordering should produce same canonical form
	id1 := ComputeFactID("r", `{"b":2,"a":1}`, 1000, "s")
	id2 := ComputeFactID("r", `{"a":1,"b":2}`, 1000, "s")
	assert.Equal(t, id1, id2, "key order should not affect ID")
}

func TestObservationWorkflow(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Simulate multiple agents observing the same codebase
	agents := []struct {
		source string
		obs    []struct {
			kind, desc, repo string
		}
	}{
		{"claude-code", []struct {
			kind, desc, repo string
		}{
			{"obstacle", "circular dependency between auth and user", "pkg/auth"},
			{"suggestion", "split Config struct into sub-configs", "internal/config"},
		}},
		{"review-agent", []struct {
			kind, desc, repo string
		}{
			{"obstacle", "circular dependency between auth and user", "pkg/auth"},
			{"pattern", "all handlers follow middleware chain pattern", "cmd/api"},
		}},
	}

	for _, agent := range agents {
		for _, o := range agent.obs {
			args, _ := json.Marshal(map[string]interface{}{
				"kind":        o.kind,
				"description": o.desc,
				"repo":        o.repo,
				"source":      agent.source,
				"status":      "raw",
				"worth":       0.5,
			})
			_, err := db.AddFact(Fact{
				Relation: "observation",
				Args:     string(args),
				Source:   agent.source,
			})
			require.NoError(t, err)
		}
	}

	// All four observations visible
	all, err := db.QueryFacts(FactFilter{Relation: "observation"})
	require.NoError(t, err)
	assert.Len(t, all, 4)

	// Corroboration: two agents flagged the same obstacle — both stored with different IDs
	var obstacles []Fact
	for _, f := range all {
		if assert.Contains(t, f.Args, "kind") {
			var args map[string]interface{}
			json.Unmarshal([]byte(f.Args), &args)
			if args["kind"] == "obstacle" {
				obstacles = append(obstacles, f)
			}
		}
	}
	assert.Len(t, obstacles, 2, "corroborated observations stored separately")
	assert.NotEqual(t, obstacles[0].ID, obstacles[1].ID)
	assert.NotEqual(t, obstacles[0].Source, obstacles[1].Source)
}

func TestQueryFactsByMultipleRelations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Store facts in different relations
	_, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"kind":"pattern","description":"test"}`,
		Source:   "agent-1",
	})
	require.NoError(t, err)

	_, err = db.AddFact(Fact{
		Relation: "depends",
		Args:     `{"from":"api","to":"db"}`,
		Source:   "scanner",
	})
	require.NoError(t, err)

	// Query by relation returns only matching facts
	obs, err := db.QueryFacts(FactFilter{Relation: "observation"})
	require.NoError(t, err)
	assert.Len(t, obs, 1)

	deps, err := db.QueryFacts(FactFilter{Relation: "depends"})
	require.NoError(t, err)
	assert.Len(t, deps, 1)

	// Non-existent relation returns empty
	empty, err := db.QueryFacts(FactFilter{Relation: "nonexistent"})
	require.NoError(t, err)
	assert.Len(t, empty, 0)
}

func TestRetractThenReassert(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Assert a fact
	f1, err := db.AddFact(Fact{
		Relation:   "depends",
		Args:       `{"from":"api","to":"db"}`,
		ValidStart: time.Now().Add(-1 * time.Hour).Unix(),
		Source:     "scanner",
	})
	require.NoError(t, err)

	// Retract it (we were wrong)
	err = db.RetractFact(f1.ID)
	require.NoError(t, err)

	// AsOfNow: gone
	facts, err := db.QueryFacts(FactFilter{Relation: "depends"})
	require.NoError(t, err)
	assert.Len(t, facts, 0)

	// Reassert with corrected data
	f2, err := db.AddFact(Fact{
		Relation: "depends",
		Args:     `{"from":"api","to":"cache"}`,
		Source:   "scanner",
	})
	require.NoError(t, err)
	assert.NotEqual(t, f1.ID, f2.ID)

	// AsOfNow: only the corrected fact
	facts, err = db.QueryFacts(FactFilter{Relation: "depends"})
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Contains(t, facts[0].Args, "cache")
}

func TestInvalidateThenQuery(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	past := time.Now().Add(-2 * time.Hour).Unix()

	// Fact was true in the past
	f, err := db.AddFact(Fact{
		Relation:   "depends",
		Args:       `{"from":"api","to":"legacy-db"}`,
		ValidStart: past,
		TxStart:    past,
		Source:     "scanner",
	})
	require.NoError(t, err)

	// Invalidate it (dependency was removed)
	err = db.InvalidateFact(f.ID)
	require.NoError(t, err)

	// AsOfNow: not valid anymore
	facts, err := db.QueryFacts(FactFilter{Relation: "depends"})
	require.NoError(t, err)
	assert.Len(t, facts, 0)

	// AsOfValid(past): was true then
	facts, err = db.QueryFacts(FactFilter{
		Relation: "depends",
		ValidAt:  &past,
	})
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Contains(t, facts[0].Args, "legacy-db")
}

func TestGetFactByPrefix(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"kind":"pattern","description":"prefix test"}`,
		Source:   "agent-1",
	})
	require.NoError(t, err)

	// Full ID works
	got, err := db.GetFactByPrefix(f.ID)
	require.NoError(t, err)
	assert.Equal(t, f.ID, got.ID)

	// Short prefix works
	got, err = db.GetFactByPrefix(f.ID[:12])
	require.NoError(t, err)
	assert.Equal(t, f.ID, got.ID)

	// Non-matching prefix fails
	_, err = db.GetFactByPrefix("zzzzzzzzzzzz")
	assert.Error(t, err)
}

func TestFactProvenance(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	prov, _ := json.Marshal(map[string]string{
		"agent":    "claude-code",
		"activity": "pr-review",
		"context":  "PR #42",
	})

	f, err := db.AddFact(Fact{
		Relation:   "observation",
		Args:       `{"kind":"suggestion","description":"split config struct"}`,
		Source:     "claude-code",
		Provenance: string(prov),
	})
	require.NoError(t, err)

	got, err := db.GetFact(f.ID)
	require.NoError(t, err)
	assert.Contains(t, got.Provenance, "pr-review")
}
