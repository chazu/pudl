package factstore

import (
	"os"
	"testing"
	"time"

	"pudl/internal/database"
)

// White-box test: seed catalog entries through the internal handle (catalog
// writes are owned by the import pipeline, not the public API), then exercise
// the read-only ListCatalog surface.
func TestListCatalog(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pudl-listcatalog-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	s, err := Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	for _, e := range []struct{ id, origin string }{{"a", "prod"}, {"b", "dev"}, {"c", "prod"}} {
		if err := s.db.AddEntry(database.CatalogEntry{
			ID:              e.id,
			StoredPath:      "/tmp/" + e.id,
			MetadataPath:    "/tmp/" + e.id + ".meta",
			ImportTimestamp: time.Unix(1, 0),
			Format:          "json",
			Origin:          e.origin,
			Schema:          "core.#Item",
			Confidence:      1.0,
			RecordCount:     1,
			SizeBytes:       1,
			CreatedAt:       time.Unix(1, 0),
			UpdatedAt:       time.Unix(1, 0),
		}); err != nil {
			t.Fatal(err)
		}
	}

	res, err := s.ListCatalog(CatalogFilter{Origin: "prod"}, CatalogQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("expected 2 prod entries, got %d", len(res.Entries))
	}
}
