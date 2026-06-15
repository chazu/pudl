package database

import (
	"os"
	"testing"
)

func newFTSTestDB(t *testing.T) *CatalogDB {
	t.Helper()
	dir, err := os.MkdirTemp("", "pudl-fts-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	db, err := NewCatalogDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func ids(facts []Fact) map[string]bool {
	m := map[string]bool{}
	for _, f := range facts {
		m[f.ID] = true
	}
	return m
}

func TestFactSearchBasics(t *testing.T) {
	db := newFTSTestDB(t)
	a, _ := db.AddFact(Fact{Relation: "observation", Args: `{"kind":"obstacle","description":"auth module has a circular dependency"}`, Source: "x"})
	b, _ := db.AddFact(Fact{Relation: "observation", Args: `{"kind":"suggestion","description":"rate limiting missing on login"}`, Source: "x"})

	// term match
	got, err := db.SearchCurrentFacts("circular", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m := ids(got); !m[a.ID] || m[b.ID] || len(m) != 1 {
		t.Errorf("search 'circular' = %v, want only %s", m, a.ID[:8])
	}

	// AND across terms
	got, _ = db.SearchCurrentFacts("rate login", "", 0)
	if m := ids(got); !m[b.ID] || len(m) != 1 {
		t.Errorf("search 'rate login' = %v, want only %s", m, b.ID[:8])
	}

	// prefix
	got, _ = db.SearchCurrentFacts("depend*", "", 0)
	if m := ids(got); !m[a.ID] {
		t.Errorf("prefix 'depend*' should match %s, got %v", a.ID[:8], m)
	}

	// JSON keys are NOT indexed
	got, _ = db.SearchCurrentFacts("description", "", 0)
	if len(got) != 0 {
		t.Errorf("JSON key 'description' should not be indexed, got %d hits", len(got))
	}

	// valid_start populated (not zero)
	got, _ = db.SearchCurrentFacts("circular", "", 0)
	if len(got) != 1 || got[0].ValidStart == 0 {
		t.Errorf("search result should carry a real valid_start, got %+v", got)
	}
}

func TestFactSearchRelationFilter(t *testing.T) {
	db := newFTSTestDB(t)
	db.AddFact(Fact{Relation: "observation", Args: `{"description":"shared connection pool"}`, Source: "x"})
	fb, _ := db.AddFact(Fact{Relation: "feedback", Args: `{"target":"t","verdict":"helpful","note":"shared pool insight"}`, Source: "x"})

	got, _ := db.SearchCurrentFacts("shared", "feedback", 0)
	if m := ids(got); !m[fb.ID] || len(m) != 1 {
		t.Errorf("relation-filtered search = %v, want only feedback %s", m, fb.ID[:8])
	}
}

func TestFactSearchRetractSync(t *testing.T) {
	db := newFTSTestDB(t)
	a, _ := db.AddFact(Fact{Relation: "observation", Args: `{"description":"orphan widget leak"}`, Source: "x"})

	if got, _ := db.SearchCurrentFacts("orphan", "", 0); len(got) != 1 {
		t.Fatalf("pre-retract: want 1 hit, got %d", len(got))
	}
	if err := db.RetractFact(a.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.SearchCurrentFacts("orphan", "", 0); len(got) != 0 {
		t.Errorf("post-retract: search should return 0, got %d (FTS not synced on retract)", len(got))
	}
}
