package datalog

import (
	"sort"
	"testing"
)

// convergenceRules mirrors the shipped bootstrap/pudl/rules/convergence.cue so the
// evaluator behavior is guarded independently of the CUE loader. (An importer-level
// test guards that the .cue file actually parses to these rules.)
func convergenceRules() []Rule {
	return []Rule{
		{
			Name: "depends_transitive_base",
			Head: Atom{Rel: "depends_transitive", Args: map[string]Term{"from": Var("A"), "to": Var("B")}},
			Body: []Atom{{Rel: "model_depends_on", Args: map[string]Term{"from": Var("A"), "to": Var("B")}}},
		},
		{
			Name: "depends_transitive_rec",
			Head: Atom{Rel: "depends_transitive", Args: map[string]Term{"from": Var("A"), "to": Var("C")}},
			Body: []Atom{
				{Rel: "model_depends_on", Args: map[string]Term{"from": Var("A"), "to": Var("B")}},
				{Rel: "depends_transitive", Args: map[string]Term{"from": Var("B"), "to": Var("C")}},
			},
		},
		{
			Name: "impacted_by",
			Head: Atom{Rel: "impacted_by", Args: map[string]Term{"changed": Var("X"), "impacted": Var("A")}},
			Body: []Atom{{Rel: "depends_transitive", Args: map[string]Term{"from": Var("A"), "to": Var("X")}}},
		},
		{
			Name: "cyclic",
			Head: Atom{Rel: "cyclic", Args: map[string]Term{"model": Var("A")}},
			Body: []Atom{{Rel: "depends_transitive", Args: map[string]Term{"from": Var("A"), "to": Var("A")}}},
		},
	}
}

func toValues(tuples []Tuple, key string) []string {
	var out []string
	for _, t := range tuples {
		if s, ok := t.Args[key].(string); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Acyclic chain: workloads -> compute -> network -> dns.
func TestConvergenceRules_TransitiveAndImpact(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "model_depends_on", `{"from":"compute","to":"network"}`)
	addTestFact(t, db, "model_depends_on", `{"from":"network","to":"dns"}`)
	addTestFact(t, db, "model_depends_on", `{"from":"workloads","to":"compute"}`)

	rules := convergenceRules()

	// what does compute depend on (transitively)? network, dns
	res, err := Evaluate(db, rules, "depends_transitive", map[string]interface{}{"from": "compute"}, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if got := toValues(res, "to"); !eq(got, []string{"dns", "network"}) {
		t.Fatalf("depends_transitive from=compute: got %v", got)
	}

	// what breaks if dns changes? network, compute, workloads
	res, err = Evaluate(db, rules, "impacted_by", map[string]interface{}{"changed": "dns"}, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if got := toValues(res, "impacted"); !eq(got, []string{"compute", "network", "workloads"}) {
		t.Fatalf("impacted_by changed=dns: got %v", got)
	}

	// no cycles in an acyclic graph
	res, err = Evaluate(db, rules, "cyclic", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("expected no cycles, got %v", res)
	}
}

func TestConvergenceRules_CycleDetection(t *testing.T) {
	db := setupTestDB(t)
	addTestFact(t, db, "model_depends_on", `{"from":"a","to":"b"}`)
	addTestFact(t, db, "model_depends_on", `{"from":"b","to":"a"}`)

	res, err := Evaluate(db, convergenceRules(), "cyclic", nil, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}
	if got := toValues(res, "model"); !eq(got, []string{"a", "b"}) {
		t.Fatalf("cyclic: got %v", got)
	}
}
