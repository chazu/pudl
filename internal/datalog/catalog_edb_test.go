package datalog

import (
	"testing"
	"time"

	"pudl/internal/database"
)

func addTestEntry(t *testing.T, db *database.CatalogDB, id, origin string) {
	t.Helper()
	entryType := "import"
	if err := db.AddEntry(database.CatalogEntry{
		ID:              id,
		StoredPath:      "/tmp/" + id,
		MetadataPath:    "/tmp/" + id + ".meta",
		ImportTimestamp: time.Unix(1, 0),
		Format:          "json",
		Origin:          origin,
		Schema:          "core.#Item",
		Confidence:      1.0,
		RecordCount:     1,
		SizeBytes:       1,
		EntryType:       &entryType,
		CreatedAt:       time.Unix(1, 0),
		UpdatedAt:       time.Unix(1, 0),
	}); err != nil {
		t.Fatal(err)
	}
}

// A rule over catalog_entry alone resolves through the view (SQL path).
func TestCatalogEDBRuleOnly(t *testing.T) {
	db := setupTestDB(t)
	addTestEntry(t, db, "a", "prod")
	addTestEntry(t, db, "b", "dev")
	addTestEntry(t, db, "c", "prod")

	rules := []Rule{{
		Name: "prod_entry",
		Head: Atom{Rel: "prod_entry", Args: map[string]Term{"id": Var("I")}},
		Body: []Atom{
			{Rel: "catalog_entry", Args: map[string]Term{"id": Var("I"), "origin": Val("prod")}},
		},
	}}

	results, err := Evaluate(db, rules, "prod_entry", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 prod entries, got %d", len(results))
	}
}

// A rule joining catalog_entry (native columns) against a fact relation (JSON
// args) on a shared variable — the core cross-source capability.
func TestCatalogEDBJoinWithFact(t *testing.T) {
	db := setupTestDB(t)
	addTestEntry(t, db, "a", "prod")
	addTestEntry(t, db, "b", "dev")
	addTestFact(t, db, "team_owns", `{"origin":"prod","team":"infra"}`)

	rules := []Rule{{
		Name: "owned",
		Head: Atom{Rel: "owned", Args: map[string]Term{"id": Var("I"), "team": Var("T")}},
		Body: []Atom{
			{Rel: "catalog_entry", Args: map[string]Term{"id": Var("I"), "origin": Var("O")}},
			{Rel: "team_owns", Args: map[string]Term{"origin": Var("O"), "team": Var("T")}},
		},
	}}

	results, err := Evaluate(db, rules, "owned", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 owned entry, got %d", len(results))
	}
	if results[0].Args["id"] != "a" || results[0].Args["team"] != "infra" {
		t.Fatalf("unexpected join result: %v", results[0].Args)
	}
}

// A recursive rule set whose base rule references catalog_entry — exercises the
// builtin override in seedBase, and catalog_entry in the recursive body
// exercises it in fixpointLoop.
func TestCatalogEDBRecursive(t *testing.T) {
	db := setupTestDB(t)
	addTestEntry(t, db, "a", "node")
	addTestEntry(t, db, "b", "node")
	addTestEntry(t, db, "c", "node")
	addTestFact(t, db, "edge", `{"from":"a","to":"b"}`)
	addTestFact(t, db, "edge", `{"from":"b","to":"c"}`)

	rules := []Rule{
		{
			Name: "reach_base",
			Head: Atom{Rel: "reach", Args: map[string]Term{"from": Var("X"), "to": Var("Y")}},
			Body: []Atom{
				{Rel: "edge", Args: map[string]Term{"from": Var("X"), "to": Var("Y")}},
			},
		},
		{
			Name: "reach_step",
			Head: Atom{Rel: "reach", Args: map[string]Term{"from": Var("X"), "to": Var("Z")}},
			Body: []Atom{
				{Rel: "edge", Args: map[string]Term{"from": Var("X"), "to": Var("Y")}},
				{Rel: "reach", Args: map[string]Term{"from": Var("Y"), "to": Var("Z")}},
				// constrain targets to known catalog ids — puts catalog_entry in
				// the recursive rule body (fixpointLoop override path).
				{Rel: "catalog_entry", Args: map[string]Term{"id": Var("Z")}},
			},
		},
	}

	results, err := Evaluate(db, rules, "reach", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	// a→b, b→c, a→c = 3 reachable pairs.
	if len(results) != 3 {
		t.Fatalf("expected 3 reachable pairs, got %d", len(results))
	}
}

// catalog_entry has no temporal columns; a query under a temporal scope must
// still resolve it (the override skips temporal filtering).
func TestCatalogEDBTemporalScope(t *testing.T) {
	db := setupTestDB(t)
	addTestEntry(t, db, "a", "prod")

	rules := []Rule{{
		Name: "prod_entry",
		Head: Atom{Rel: "prod_entry", Args: map[string]Term{"id": Var("I")}},
		Body: []Atom{
			{Rel: "catalog_entry", Args: map[string]Term{"id": Var("I"), "origin": Val("prod")}},
		},
	}}

	now := time.Now().Unix()
	results, err := Evaluate(db, rules, "prod_entry", nil, TemporalScope{ValidAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 entry under temporal scope, got %d", len(results))
	}
}

// The datalog override map must reference only relation names the database
// package reserves, so reserved-name enforcement and overrides stay aligned.
func TestBuiltinEDBTablesAreReserved(t *testing.T) {
	for rel := range builtinEDBTables {
		if !database.IsReservedRelation(rel) {
			t.Errorf("builtin EDB relation %q is not reserved in database package", rel)
		}
	}
}
