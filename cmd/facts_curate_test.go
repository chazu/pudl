package cmd

import (
	"encoding/json"
	"testing"

	"github.com/chazu/pudl/internal/database"
)

func TestPromoteFactRecordsLineage(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	f, err := db.AddFact(database.Fact{
		Relation: "observation",
		Args:     `{"kind":"fact","description":"x","status":"raw","worth":0.5}`,
		Source:   "a",
	})
	if err != nil {
		t.Fatal(err)
	}

	oldID, newID, err := promoteFact(db, f.ID, "reviewed", "")
	if err != nil {
		t.Fatalf("promoteFact: %v", err)
	}
	if oldID != f.ID {
		t.Errorf("oldID = %s, want %s", oldID, f.ID)
	}

	nf, err := db.GetFact(newID)
	if err != nil {
		t.Fatal(err)
	}
	var a map[string]interface{}
	if err := json.Unmarshal([]byte(nf.Args), &a); err != nil {
		t.Fatal(err)
	}
	if a["status"] != "reviewed" {
		t.Errorf("status = %v, want reviewed", a["status"])
	}
	if a["prevVersion"] != f.ID {
		t.Errorf("prevVersion = %v, want %s", a["prevVersion"], f.ID)
	}

	// Lineage of the new version includes both the new and original IDs.
	ids := observationLineage(db, *nf)
	if len(ids) != 2 || ids[0] != newID || ids[1] != f.ID {
		t.Errorf("lineage = %v, want [%s %s]", ids, newID, f.ID)
	}
}

func TestPromoteFactRejectsIllegalTransition(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	f, _ := db.AddFact(database.Fact{
		Relation: "observation",
		Args:     `{"kind":"fact","description":"x","status":"raw","worth":0.5}`,
		Source:   "a",
	})
	if _, _, err := promoteFact(db, f.ID, "promoted", ""); err == nil {
		t.Fatal("expected illegal transition raw → promoted to error")
	}
}
