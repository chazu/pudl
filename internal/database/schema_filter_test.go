package database

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

func TestSchemaFilterAnchorsDefinitionNames(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	for i, schema := range []string{"pudl/gcp.#Route", "pudl/gcp.#Router", "pudl/aws.#EC2Instance", "pudl/x_y.#A"} {
		if err := db.AddEntry(CatalogEntry{
			ID: fmt.Sprintf("%064d", i), StoredPath: "/tmp/x", MetadataPath: "/tmp/x.meta",
			ImportTimestamp: time.Now(), Format: "json", Origin: "test", Schema: schema, Confidence: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}

	cases := map[string][]string{
		"pudl/gcp.#Route":  {"pudl/gcp.#Route"},
		"gcp.#Route":       {"pudl/gcp.#Route"},
		"#Route":           {"pudl/gcp.#Route"},
		"aws.#EC2Instance": {"pudl/aws.#EC2Instance"},
		"Route":            {"pudl/gcp.#Route", "pudl/gcp.#Router"},
		"x%y":              nil,
		"x_y":              {"pudl/x_y.#A"},
		"#Rout":            nil,
	}
	for filter, want := range cases {
		res, err := db.QueryEntries(FilterOptions{Schema: filter}, QueryOptions{})
		if err != nil {
			t.Fatalf("%s: %v", filter, err)
		}
		var got []string
		for _, e := range res.Entries {
			got = append(got, e.Schema)
		}
		sort.Strings(got)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("--schema %q = %v, want %v", filter, got, want)
		}
	}
}
