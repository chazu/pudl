package datalog

import (
	"fmt"
	"os"
	"testing"

	"github.com/chazu/pudl/internal/database"
)

// Benchmarks for the evaluation routes a workspace exercises: layered
// (acyclic) derived relations, a filtered transitive closure next to an
// unrelated recursive relation, and a model check that matches nothing in a
// workspace that also has recursive rules.

func benchDB(b *testing.B) *database.CatalogDB {
	b.Helper()
	dir, err := os.MkdirTemp("", "pudl-datalog-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { os.RemoveAll(dir) })
	db, err := database.NewCatalogDB(dir)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { db.Close() })
	return db
}

func benchFact(b *testing.B, db *database.CatalogDB, relation, args string) {
	b.Helper()
	if _, err := db.AddFact(database.Fact{Relation: relation, Args: args, ValidStart: 1, TxStart: 1}); err != nil {
		b.Fatal(err)
	}
}

func benchRules(b *testing.B, src string) []Rule {
	b.Helper()
	return mustParse(b, src)
}

// edges adds a chain n0 -> n1 -> ... of the given length under relation.
func edges(b *testing.B, db *database.CatalogDB, relation, prefix string, length int) {
	for i := 0; i < length; i++ {
		benchFact(b, db, relation, fmt.Sprintf(`{"from":"%s%d","to":"%s%d"}`, prefix, i, prefix, i+1))
	}
}

const closureRules = `
depends_transitive_base: {
	head: {rel: "depends_transitive", args: {from: "$A", to: "$B"}}
	body: [{rel: "model_depends_on", args: {from: "$A", to: "$B"}}]
}
depends_transitive_rec: {
	head: {rel: "depends_transitive", args: {from: "$A", to: "$C"}}
	body: [{rel: "model_depends_on", args: {from: "$A", to: "$B"}}, {rel: "depends_transitive", args: {from: "$B", to: "$C"}}]
}
reach_base: {
	head: {rel: "reach", args: {from: "$A", to: "$B"}}
	body: [{rel: "link", args: {from: "$A", to: "$B"}}]
}
reach_rec: {
	head: {rel: "reach", args: {from: "$A", to: "$C"}}
	body: [{rel: "link", args: {from: "$A", to: "$B"}}, {rel: "reach", args: {from: "$B", to: "$C"}}]
}
`

func BenchmarkLayeredRules(b *testing.B) {
	db := benchDB(b)
	edges(b, db, "edge", "n", 300)
	rules := benchRules(b, `
l1: {
	head: {rel: "l1", args: {from: "$A", to: "$B"}}
	body: [{rel: "edge", args: {from: "$A", to: "$B"}}]
}
l2: {
	head: {rel: "l2", args: {from: "$A", to: "$C"}}
	body: [{rel: "l1", args: {from: "$A", to: "$B"}}, {rel: "edge", args: {from: "$B", to: "$C"}}]
}
l3: {
	head: {rel: "l3", args: {from: "$A", to: "$D"}}
	body: [{rel: "l2", args: {from: "$A", to: "$C"}}, {rel: "edge", args: {from: "$C", to: "$D"}}]
}
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := Evaluate(db, rules, "l3", nil, TemporalScope{})
		if err != nil || len(got) != 298 {
			b.Fatalf("l3: %d tuples, err %v", len(got), err)
		}
	}
}

func BenchmarkTransitiveClosureFiltered(b *testing.B) {
	db := benchDB(b)
	edges(b, db, "model_depends_on", "m", 30)
	edges(b, db, "link", "x", 90)
	rules := benchRules(b, closureRules)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := Evaluate(db, rules, "depends_transitive", map[string]interface{}{"from": "m0"}, TemporalScope{})
		if err != nil || len(got) != 30 {
			b.Fatalf("depends_transitive: %d tuples, err %v", len(got), err)
		}
	}
}

func BenchmarkCheckZeroRows(b *testing.B) {
	db := benchDB(b)
	edges(b, db, "model_depends_on", "m", 30)
	edges(b, db, "link", "x", 90)
	for i := 0; i < 50; i++ {
		benchFact(b, db, "svc", fmt.Sprintf(`{"id":"s%d","status":"ok"}`, i))
	}
	rules := benchRules(b, closureRules+`
failed_service: {
	head: {rel: "failed_service", args: {id: "$S"}}
	body: [{rel: "svc", args: {id: "$S", status: "failed"}}]
}
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := Evaluate(db, rules, "failed_service", nil, TemporalScope{})
		if err != nil || len(got) != 0 {
			b.Fatalf("failed_service: %d tuples, err %v", len(got), err)
		}
	}
}
