package repair

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
)

// TestInferenceRepairSkipsExplicitAssignments: the repair re-checks the
// entries it is handed, so an explicit --schema import is left alone even
// when the drift list names it; --include-explicit re-infers it.
func TestInferenceRepairSkipsExplicitAssignments(t *testing.T) {
	dir, inferrer := identityFixture(t)
	db, err := database.NewCatalogDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	dataPath := write("data.json", `{"id":"x1"}`)
	metas := map[string]string{
		"inferred": write("inferred.meta", `{"schema_info":{"validation_status":"auto-assigned"}}`),
		// Recorded before intended_schema was: the status alone is explicit.
		"legacy":   write("legacy.meta", `{"schema_info":{"validation_status":"validated"}}`),
		"intended": write("intended.meta", `{"schema_info":{"validation_status":"unverified","intended_schema":"unknown.#CatchAll"}}`),
	}
	var ids []string
	for id, meta := range metas {
		entry := database.CatalogEntry{ID: id, Schema: "unknown.#CatchAll", StoredPath: dataPath, MetadataPath: meta, Format: "json", ImportTimestamp: time.Now()}
		if err := db.AddEntry(entry); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	env := &Env{DB: db, Inferrer: inferrer, Options: Options{DriftScanned: true, DriftEntryIDs: ids}}

	applied := Run(context.Background(), inferenceDrift, env, true, false)
	if applied.Changed != 1 || applied.Pending != 1 {
		t.Fatalf("apply = %+v, want only the inferred entry changed", applied)
	}
	for id := range metas {
		got, _ := db.GetEntry(id)
		if moved := got.Schema != "unknown.#CatchAll"; moved != (id == "inferred") {
			t.Errorf("%s schema = %s", id, got.Schema)
		}
	}

	env.Options = Options{IncludeExplicit: true, EntryID: "legacy"}
	if again := Run(context.Background(), inferenceDrift, env, true, false); again.Changed != 1 {
		t.Fatalf("--include-explicit apply = %+v, want the explicit entry re-inferred", again)
	}
}
