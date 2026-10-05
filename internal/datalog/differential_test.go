package datalog

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/database"
)

// TestDifferentialAgainstOracle generates random rule sets over a small value
// domain — joins, constants, several rules per relation, recursion and mutual
// recursion — and checks that Evaluate agrees with the frozen pre-stratification
// evaluator (oracle_test.go) on every derived relation, with and without a
// constraint. Each seed's relations are prefixed, so one database serves all.
func TestDifferentialAgainstOracle(t *testing.T) {
	seeds := 300
	if s := os.Getenv("PUDL_DATALOG_DIFF_SEEDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			seeds = n
		}
	}
	if testing.Short() {
		seeds = 40
	}
	db := setupTestDB(t)

	var compared, nonEmpty, recursivePrograms int
	for seed := 0; seed < seeds; seed++ {
		g := newRuleGen(uint64(seed))
		g.addFacts(t, db)
		rules := g.rules()
		for _, rel := range g.derived {
			for _, constraints := range []map[string]interface{}{nil, {"a": g.value(0)}} {
				want, wantErr := oracleEvaluate(db, rules, rel, constraints, TemporalScope{})
				got, gotErr := Evaluate(db, rules, rel, constraints, TemporalScope{})
				if (wantErr != nil) != (gotErr != nil) {
					t.Fatalf("seed %d %s %v: error mismatch: oracle %v, evaluate %v\n%s", seed, rel, constraints, wantErr, gotErr, g.describe())
				}
				if wantErr != nil {
					continue
				}
				if w, gt := tupleSet(want), tupleSet(got); w != gt {
					t.Fatalf("seed %d %s %v: results differ\noracle:   %s\nevaluate: %s\n%s", seed, rel, constraints, w, gt, g.describe())
				}
				compared++
				if len(want) > 0 {
					nonEmpty++
				}
			}
		}
		if hasCycle(rules) {
			recursivePrograms++
		}
	}
	t.Logf("%d comparisons, %d non-empty; %d of %d programs recursive", compared, nonEmpty, recursivePrograms, seeds)
	if nonEmpty < compared/4 || recursivePrograms < seeds/4 {
		t.Fatalf("generator too weak: %d/%d non-empty, %d/%d recursive", nonEmpty, compared, recursivePrograms, seeds)
	}
}

// hasCycle reports whether any derived relation can reach itself.
func hasCycle(rules []Rule) bool {
	for _, r := range rules {
		for _, a := range r.Body {
			if dependencyClosure(rules, a.Rel)[r.Head.Rel] {
				return true
			}
		}
	}
	return false
}

// ruleGen builds one random program. EDB relations e0..e2 and derived
// relations d0..d3 all have keys a and b, over values v0..v3.
type ruleGen struct {
	r       *rand.Rand
	prefix  string
	derived []string
	edb     []string
	program []Rule
}

func newRuleGen(seed uint64) *ruleGen {
	g := &ruleGen{r: rand.New(rand.NewPCG(seed, 0x5eed)), prefix: fmt.Sprintf("s%d_", seed)}
	for i := 0; i < 3; i++ {
		g.edb = append(g.edb, fmt.Sprintf("%se%d", g.prefix, i))
	}
	for i := 0; i < 4; i++ {
		g.derived = append(g.derived, fmt.Sprintf("%sd%d", g.prefix, i))
	}
	return g
}

func (g *ruleGen) value(i int) string { return "v" + strconv.Itoa(i) }

func (g *ruleGen) addFacts(t *testing.T, db *database.CatalogDB) {
	t.Helper()
	for _, rel := range g.edb {
		for n := 0; n < 6; n++ {
			args := fmt.Sprintf(`{"a":%q,"b":%q}`, g.value(g.r.IntN(4)), g.value(g.r.IntN(4)))
			if _, err := db.AddFact(database.Fact{Relation: rel, Args: args, ValidStart: 1, TxStart: 1}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func (g *ruleGen) rules() []Rule {
	for i, head := range g.derived {
		n := 1 + g.r.IntN(3)
		for j := 0; j < n; j++ {
			if r, ok := g.rule(fmt.Sprintf("%sr%d_%d", g.prefix, i, j), head); ok {
				g.program = append(g.program, r)
			}
		}
	}
	return g.program
}

func (g *ruleGen) rule(name, head string) (Rule, bool) {
	vars := []string{"$X", "$Y", "$Z"}
	term := func() Term {
		if g.r.IntN(6) == 0 {
			return Val(g.value(g.r.IntN(4)))
		}
		return Var(vars[g.r.IntN(len(vars))])
	}
	var body []Atom
	for k := 0; k < 1+g.r.IntN(2); k++ {
		pool := g.edb
		if g.r.IntN(2) == 0 {
			pool = g.derived
		}
		body = append(body, Atom{Rel: pool[g.r.IntN(len(pool))], Args: map[string]Term{"a": term(), "b": term()}})
	}
	bound := map[string]bool{}
	for _, a := range body {
		for _, tm := range a.Args {
			if tm.IsVariable() {
				bound[tm.Variable] = true
			}
		}
	}
	headTerm := func() Term {
		if g.r.IntN(5) == 0 || len(bound) == 0 {
			return Val(g.value(g.r.IntN(4)))
		}
		for _, v := range vars {
			if bound[v] && g.r.IntN(2) == 0 {
				return Var(v)
			}
		}
		for _, v := range vars {
			if bound[v] {
				return Var(v)
			}
		}
		return Val(g.value(0))
	}
	r := Rule{Name: name, Head: Atom{Rel: head, Args: map[string]Term{"a": headTerm(), "b": headTerm()}}, Body: body}
	return r, checkRule(r) == nil
}

func (g *ruleGen) describe() string {
	var b strings.Builder
	for _, r := range g.program {
		fmt.Fprintf(&b, "  %s(%s) :- ", r.Head.Rel, describeArgs(r.Head.Args))
		var parts []string
		for _, a := range r.Body {
			parts = append(parts, fmt.Sprintf("%s(%s)", a.Rel, describeArgs(a.Args)))
		}
		b.WriteString(strings.Join(parts, ", ") + "\n")
	}
	return b.String()
}

func describeArgs(args map[string]Term) string {
	var parts []string
	for _, k := range sortedArgKeys(args) {
		parts = append(parts, k+"="+args[k].String())
	}
	return strings.Join(parts, ",")
}

// tupleSet renders tuples as a sorted, deduplicated string for comparison.
func tupleSet(tuples []Tuple) string {
	seen := map[string]bool{}
	for _, t := range tuples {
		b, _ := json.Marshal(t.Args)
		seen[string(b)] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return fmt.Sprintf("%d %s", len(tuples), strings.Join(out, " "))
}
