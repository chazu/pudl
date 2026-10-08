package database

import (
	"testing"
)

func currentArgs(t *testing.T, db *CatalogDB, relation string) []string {
	t.Helper()
	facts, err := db.QueryFacts(FactFilter{Relation: relation})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range facts {
		out = append(out, f.Args)
	}
	return out
}

func reconcile(t *testing.T, db *CatalogDB, want []Fact, mode ProjectionMode) (int, int) {
	t.Helper()
	var added, closed int
	err := db.WithCatalogTx(func(tx *CatalogTx) error {
		var err error
		added, closed, err = tx.ReconcileProjection(ProjectionSource("rid1"), want, mode)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return added, closed
}

func TestReconcileProjectionRevertRestoresFacts(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	a := []Fact{{Relation: "fw", Args: `{"name":"x","open":true}`}}
	b := []Fact{{Relation: "fw", Args: `{"name":"x","open":false}`}}

	if added, _ := reconcile(t, db, a, ProjectionObservation); added != 1 {
		t.Fatalf("A added %d", added)
	}
	if added, closed := reconcile(t, db, a, ProjectionObservation); added != 0 || closed != 0 {
		t.Fatalf("A again: added %d closed %d, want a no-op", added, closed)
	}
	if added, closed := reconcile(t, db, b, ProjectionObservation); added != 1 || closed != 1 {
		t.Fatalf("B: added %d closed %d", added, closed)
	}
	// Reverting to A within the same second collides with A's closed fact ID;
	// the facts must come back anyway.
	reconcile(t, db, a, ProjectionObservation)
	got := currentArgs(t, db, "fw")
	if len(got) != 1 || got[0] != `{"name":"x","open":true}` {
		t.Fatalf("after revert current = %v", got)
	}

	if _, closed := reconcile(t, db, nil, ProjectionCorrection); closed != 1 {
		t.Fatalf("correction to empty closed %d", closed)
	}
	if got := currentArgs(t, db, "fw"); len(got) != 0 {
		t.Fatalf("after correction current = %v", got)
	}
}

func TestProjectionSourceIsReserved(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	if _, err := db.AddFact(Fact{Relation: "r", Args: `{"a":1}`, Source: "projection:x"}); err == nil {
		t.Fatal("general writers must not use the projection source prefix")
	}
}
