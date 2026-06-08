package database

import (
	"testing"
	"time"
)

func TestAddFactRejectsReservedRelation(t *testing.T) {
	db := newTestDB(t)
	_, err := db.AddFact(Fact{Relation: CatalogEntryRelation, Args: `{"id":"x"}`})
	if err == nil {
		t.Fatalf("expected AddFact to reject reserved relation %q", CatalogEntryRelation)
	}
}

func TestAddFactAllowsNormalRelation(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.AddFact(Fact{Relation: "depends", Args: `{"from":"a","to":"b"}`}); err != nil {
		t.Fatalf("AddFact should allow non-reserved relation: %v", err)
	}
}

// The catalog_entry_edb view exists and projects catalog_entries rows.
func TestCatalogEntryViewProjectsEntries(t *testing.T) {
	db := newTestDB(t)
	if err := db.AddEntry(CatalogEntry{
		ID:              "a",
		StoredPath:      "/tmp/a",
		MetadataPath:    "/tmp/a.meta",
		ImportTimestamp: time.Unix(1, 0),
		Format:          "json",
		Origin:          "prod",
		Schema:          "core.#Item",
		Confidence:      1.0,
		RecordCount:     1,
		SizeBytes:       1,
		CreatedAt:       time.Unix(1, 0),
		UpdatedAt:       time.Unix(1, 0),
	}); err != nil {
		t.Fatal(err)
	}

	var id, origin string
	row := db.DB().QueryRow("SELECT id, origin FROM " + CatalogEntryView + " WHERE id = ?", "a")
	if err := row.Scan(&id, &origin); err != nil {
		t.Fatalf("view query failed: %v", err)
	}
	if id != "a" || origin != "prod" {
		t.Fatalf("unexpected view row: id=%q origin=%q", id, origin)
	}
}
