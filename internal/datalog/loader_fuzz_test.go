package datalog

import (
	"os"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
)

// FuzzParseRulesCompile checks the rule loader never panics, and that every
// rule it accepts (LoadErr == nil) compiles to SQL SQLite can prepare against
// a real catalog, under both the current and an as-of temporal scope.
func FuzzParseRulesCompile(f *testing.F) {
	for _, seed := range []string{
		`r: { head: { rel: "flagged", args: { id: "$X", severity: "high" } }, body: [{ rel: "svc", args: { id: "$X" } }] }`,
		`r: { head: { rel: "n", args: { c: "count($X)" } }, body: [{ rel: "svc", args: { id: "$X" } }] }`,
		`r: { head: { rel: "p", args: { a: "$A", b: "$B" } }, body: [{ rel: "e", args: { from: "$A", to: "$C" } }, { rel: "e", args: { from: "$C", to: "$B" } }] }`,
		`r: { head: { rel: "c", args: { id: "$I" } }, body: [{ rel: "catalog_entry", args: { id: "$I", schema: "pudl/core.#Item" } }] }`,
		`r: { head: { rel: "q", args: { "app-name": "$X" } }, body: [{ rel: "s", args: { "app.name": "$X", "n": 3, "ok": true } }] }`,
		`r: { head: { rel: "g", args: { x: "$X" } }, body: [{ rel: "s", args: { x: "$X", y: ">5" } }] }`,
		`r: { head: { rel: "bad", args: { id: "$X" } }, body: [{ args: { id: "$X" } }] }`,
		`r: { head: { rel: "unbound", args: { id: "$Y" } }, body: [{ rel: "s", args: { id: "$X" } }] }`,
		`x: 1`,
		`r: { head: "nope" }`,
	} {
		f.Add(seed)
	}

	tmpDir, err := os.MkdirTemp("", "pudl-datalog-fuzz-*")
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { os.RemoveAll(tmpDir) })
	db, err := database.NewCatalogDB(tmpDir)
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { db.Close() })

	asOf := time.Now().Unix()
	scopes := []TemporalScope{{}, {ValidAt: &asOf, TxAt: &asOf}}

	f.Fuzz(func(t *testing.T, source string) {
		rules, err := ParseRulesFromSource(source)
		if err != nil {
			return // source that is not CUE is a reported error, not a panic
		}
		for _, rule := range rules {
			if rule.LoadErr != nil {
				continue
			}
			for _, scope := range scopes {
				compiled, err := CompileWithOptions(rule, scope, CompileOptions{TableOverrides: builtinEDBTables})
				if err != nil {
					t.Fatalf("accepted rule %q does not compile: %v\nsource: %s", rule.Name, err, source)
				}
				stmt, err := db.DB().Prepare(compiled.SQL)
				if err != nil {
					t.Fatalf("accepted rule %q compiles to SQL SQLite rejects: %v\nSQL: %s\nsource: %s", rule.Name, err, compiled.SQL, source)
				}
				stmt.Close()
			}
		}
	})
}
