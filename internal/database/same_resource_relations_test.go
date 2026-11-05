package database

import (
	"testing"
)

// same_resource_key is written only by projection, from a schema's
// `_pudl.same_resource`; same_resource is only derived by rules.
func TestSameResourceRelationsAreReserved(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	for _, rel := range []string{SameResourceRelation, SameResourceKeyRelation} {
		if !IsReservedRelation(rel) {
			t.Errorf("%s is not reserved", rel)
		}
		if _, err := db.AddFact(Fact{Relation: rel, Args: `{"a":"x","b":"y","kind":"k8s.cluster"}`, Source: "user"}); err == nil {
			t.Errorf("AddFact accepted a user fact under %s", rel)
		}
	}

	key := Fact{Relation: SameResourceKeyRelation, Args: `{"entry_id":"e1","resource_id":"rid1","kind":"k8s.cluster","key":"{\"host\":\"10.0.0.2\"}"}`}
	if added, _ := reconcile(t, db, []Fact{key}, ProjectionObservation); added != 1 {
		t.Fatalf("projection added %d same_resource_key facts, want 1", added)
	}
	if got := currentArgs(t, db, SameResourceKeyRelation); len(got) != 1 {
		t.Fatalf("current same_resource_key facts = %v", got)
	}

	// An evidence view re-projects a snapshot's records under the snapshot's source.
	evidenceKey := key
	evidenceKey.Source = SnapshotEvidenceSource("snap1")
	if _, err := db.AddFact(evidenceKey); err != nil {
		t.Fatalf("evidence view key rejected: %v", err)
	}

	link := Fact{Relation: SameResourceRelation, Args: `{"a":"x","b":"y","kind":"k8s.cluster"}`}
	err := db.WithCatalogTx(func(tx *CatalogTx) error {
		_, _, err := tx.ReconcileProjection(ProjectionSource("rid1"), []Fact{key, link}, ProjectionObservation)
		return err
	})
	if err == nil {
		t.Fatal("projection wrote a same_resource fact; links are derived only")
	}
}
