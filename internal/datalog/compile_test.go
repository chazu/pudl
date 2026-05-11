package datalog

import (
	"strings"
	"testing"
)

func TestCompileSingleAtom(t *testing.T) {
	rule := Rule{
		Name: "find_depends",
		Head: Atom{Rel: "result", Args: map[string]Term{
			"from": Var("S"),
			"to":   Var("D"),
		}},
		Body: []Atom{
			{Rel: "depends", Args: map[string]Term{
				"from": Var("S"),
				"to":   Var("D"),
			}},
		},
	}

	cq, err := Compile(rule, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(cq.SQL, "current_facts t0") {
		t.Error("expected current_facts t0 in FROM")
	}
	if !strings.Contains(cq.SQL, "t0.relation = ?") {
		t.Error("expected relation binding")
	}
	if !strings.Contains(cq.SQL, "SELECT DISTINCT") {
		t.Error("expected DISTINCT")
	}
	if len(cq.Params) != 1 || cq.Params[0] != "depends" {
		t.Errorf("expected params [depends], got %v", cq.Params)
	}
}

func TestCompileTwoAtomJoin(t *testing.T) {
	rule := Rule{
		Name: "at_risk",
		Head: Atom{Rel: "at_risk", Args: map[string]Term{
			"service": Var("S"),
		}},
		Body: []Atom{
			{Rel: "depends", Args: map[string]Term{
				"from": Var("S"),
				"to":   Var("D"),
			}},
			{Rel: "observation", Args: map[string]Term{
				"target": Var("D"),
				"kind":   Val("unhealthy"),
			}},
		},
	}

	cq, err := Compile(rule, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(cq.SQL, "current_facts t0, current_facts t1") {
		t.Error("expected two table aliases")
	}
	if !strings.Contains(cq.SQL, "json_extract(t0.args, '$.to') = json_extract(t1.args, '$.target')") {
		t.Errorf("expected equi-join on shared variable $D, got:\n%s", cq.SQL)
	}
	if !strings.Contains(cq.SQL, "json_extract(t1.args, '$.kind') = ?") {
		t.Error("expected ground term binding for kind")
	}

	expectedParams := []interface{}{"depends", "observation", "unhealthy"}
	if len(cq.Params) != len(expectedParams) {
		t.Fatalf("expected %d params, got %d: %v", len(expectedParams), len(cq.Params), cq.Params)
	}
	for i, p := range expectedParams {
		if cq.Params[i] != p {
			t.Errorf("param[%d]: expected %v, got %v", i, p, cq.Params[i])
		}
	}
}

func TestCompileGroundTerms(t *testing.T) {
	rule := Rule{
		Name: "healthy_obs",
		Head: Atom{Rel: "healthy", Args: map[string]Term{
			"target": Var("T"),
		}},
		Body: []Atom{
			{Rel: "observation", Args: map[string]Term{
				"target": Var("T"),
				"kind":   Val("healthy"),
			}},
		},
	}

	cq, err := Compile(rule, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(cq.SQL, "json_extract(t0.args, '$.kind') = ?") {
		t.Error("expected ground term filter")
	}

	foundHealthy := false
	for _, p := range cq.Params {
		if p == "healthy" {
			foundHealthy = true
		}
	}
	if !foundHealthy {
		t.Error("expected 'healthy' in params")
	}
}

func TestCompileTemporalScope(t *testing.T) {
	validAt := int64(1000)
	rule := Rule{
		Name: "temporal_test",
		Head: Atom{Rel: "r", Args: map[string]Term{"x": Var("X")}},
		Body: []Atom{
			{Rel: "obs", Args: map[string]Term{"x": Var("X")}},
		},
	}

	cq, err := Compile(rule, TemporalScope{ValidAt: &validAt})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(cq.SQL, "facts t0") {
		t.Error("expected facts table for temporal query")
	}
	if strings.Contains(cq.SQL, "current_facts") {
		t.Error("should not use current_facts for temporal query")
	}
	if !strings.Contains(cq.SQL, "t0.valid_start <= ?") {
		t.Error("expected valid_start filter")
	}
	if !strings.Contains(cq.SQL, "t0.valid_end IS NULL OR t0.valid_end > ?") {
		t.Error("expected valid_end filter")
	}
	if !strings.Contains(cq.SQL, "t0.tx_end IS NULL") {
		t.Error("expected tx_end IS NULL for valid-only temporal scope")
	}
}

func TestCompileTemporalScopeBoth(t *testing.T) {
	validAt := int64(1000)
	txAt := int64(2000)
	rule := Rule{
		Name: "bitemporal",
		Head: Atom{Rel: "r", Args: map[string]Term{"x": Var("X")}},
		Body: []Atom{
			{Rel: "obs", Args: map[string]Term{"x": Var("X")}},
		},
	}

	cq, err := Compile(rule, TemporalScope{ValidAt: &validAt, TxAt: &txAt})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(cq.SQL, "t0.tx_start <= ?") {
		t.Error("expected tx_start filter")
	}
	if !strings.Contains(cq.SQL, "t0.tx_end IS NULL OR t0.tx_end > ?") {
		t.Error("expected tx_end filter")
	}
}

func TestCompileHeadSubsetProjection(t *testing.T) {
	rule := Rule{
		Name: "project_test",
		Head: Atom{Rel: "result", Args: map[string]Term{
			"source": Var("S"),
		}},
		Body: []Atom{
			{Rel: "depends", Args: map[string]Term{
				"from": Var("S"),
				"to":   Var("D"),
			}},
		},
	}

	cq, err := Compile(rule, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(cq.SQL, `AS "source"`) {
		t.Errorf("expected source projection, got:\n%s", cq.SQL)
	}
	if strings.Contains(cq.SQL, `AS "to"`) {
		t.Error("should not project 'to' since it's not in head")
	}
}

func TestCompileThreeWayJoin(t *testing.T) {
	rule := Rule{
		Name: "three_way",
		Head: Atom{Rel: "chain", Args: map[string]Term{
			"start": Var("A"),
			"end":   Var("C"),
		}},
		Body: []Atom{
			{Rel: "edge", Args: map[string]Term{"from": Var("A"), "to": Var("B")}},
			{Rel: "edge", Args: map[string]Term{"from": Var("B"), "to": Var("C")}},
			{Rel: "node", Args: map[string]Term{"name": Var("B"), "active": Val(true)}},
		},
	}

	cq, err := Compile(rule, TemporalScope{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(cq.SQL, "t0, current_facts t1, current_facts t2") {
		t.Error("expected three table aliases")
	}

	joinCount := strings.Count(cq.SQL, "json_extract(t") - 2 // subtract the SELECT projections
	if joinCount < 3 {
		t.Errorf("expected multiple join/filter conditions, got SQL:\n%s", cq.SQL)
	}
}

func TestCompileEmptyBodyError(t *testing.T) {
	rule := Rule{
		Name: "bad",
		Head: Atom{Rel: "r", Args: map[string]Term{"x": Var("X")}},
		Body: []Atom{},
	}

	_, err := Compile(rule, TemporalScope{})
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestCompileUnboundHeadVarError(t *testing.T) {
	rule := Rule{
		Name: "bad",
		Head: Atom{Rel: "r", Args: map[string]Term{"x": Var("MISSING")}},
		Body: []Atom{
			{Rel: "obs", Args: map[string]Term{"y": Var("Y")}},
		},
	}

	_, err := Compile(rule, TemporalScope{})
	if err == nil {
		t.Error("expected error for unbound head variable")
	}
}
