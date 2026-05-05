package database

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func newItemSchemasDB(t *testing.T) *CatalogDB {
	t.Helper()
	s := NewDatabaseTestSuite(t)
	db, err := NewCatalogDB(s.TempDir)
	if err != nil {
		t.Fatalf("NewCatalogDB: %v", err)
	}
	s.cleanupFns = append(s.cleanupFns, func() error { return db.Close() })
	return db
}

func TestItemSchemas_AddAndList(t *testing.T) {
	db := newItemSchemasDB(t)
	rows := []ItemSchema{
		{ItemID: "abc-123", SchemaRef: "mu/aws@v1#EC2Instance", Status: ItemSchemaStatusDeclared},
		{ItemID: "abc-123", SchemaRef: "cloud/compute@v1#Instance", Status: ItemSchemaStatusInferred},
	}
	for _, r := range rows {
		if err := db.AddItemSchema(r); err != nil {
			t.Fatalf("AddItemSchema: %v", err)
		}
	}
	got, err := db.ListItemSchemas("abc-123")
	if err != nil {
		t.Fatalf("ListItemSchemas: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
}

func TestItemSchemas_UpsertOnConflict(t *testing.T) {
	db := newItemSchemasDB(t)
	if err := db.AddItemSchema(ItemSchema{
		ItemID: "x", SchemaRef: "mu/aws@v1", Status: ItemSchemaStatusUnresolved,
		ClassifiedAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddItemSchema(ItemSchema{
		ItemID: "x", SchemaRef: "mu/aws@v1", Status: ItemSchemaStatusDeclared,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListItemSchemas("x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1 (upsert should not duplicate)", len(got))
	}
	if got[0].Status != ItemSchemaStatusDeclared {
		t.Errorf("status = %q, want declared (after upgrade)", got[0].Status)
	}
}

func TestItemSchemas_ListUnresolved(t *testing.T) {
	db := newItemSchemasDB(t)
	for _, r := range []ItemSchema{
		{ItemID: "a", SchemaRef: "mu/aws@v1", Status: ItemSchemaStatusUnresolved},
		{ItemID: "b", SchemaRef: "mu/aws@v1", Status: ItemSchemaStatusUnresolved},
		{ItemID: "c", SchemaRef: "mu/k8s@v1", Status: ItemSchemaStatusUnresolved},
		{ItemID: "d", SchemaRef: "mu/aws@v1", Status: ItemSchemaStatusDeclared},
	} {
		if err := db.AddItemSchema(r); err != nil {
			t.Fatal(err)
		}
	}
	all, err := db.ListUnresolvedItemSchemas("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("ListUnresolvedItemSchemas all: got %d, want 3", len(all))
	}
	awsOnly, err := db.ListUnresolvedItemSchemas("mu/aws@v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(awsOnly) != 2 {
		t.Errorf("ListUnresolvedItemSchemas mu/aws@v1: got %d, want 2", len(awsOnly))
	}
}

func TestItemSchemas_Delete(t *testing.T) {
	db := newItemSchemasDB(t)
	if err := db.AddItemSchema(ItemSchema{ItemID: "x", SchemaRef: "r", Status: "unresolved"}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteItemSchema("x", "r"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := db.DeleteItemSchema("x", "r"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second delete: got %v, want ErrNoRows", err)
	}
}
