package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/datalog"
)

func TestCheckPasses(t *testing.T) {
	cases := []struct {
		expect string
		count  int
		want   bool
	}{
		{"empty", 0, true},
		{"empty", 1, false},
		{"empty", 5, false},
		{"nonempty", 0, false},
		{"nonempty", 1, true},
		{"bogus", 0, false}, // unknown expect never passes
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, Passes(tc.expect, tc.count),
			"Passes(%q,%d)", tc.expect, tc.count)
	}
}

func TestHeadExposesRunID(t *testing.T) {
	rules := []datalog.Rule{
		{
			Name: "scoped",
			Head: datalog.Atom{Rel: "scoped_check", Args: map[string]datalog.Term{
				"run_id": datalog.Var("$R"), "target": datalog.Var("$T"),
			}},
		},
		{
			Name: "global",
			Head: datalog.Atom{Rel: "global_check", Args: map[string]datalog.Term{
				"target": datalog.Var("$T"),
			}},
		},
		{
			Name: "ground",
			Head: datalog.Atom{Rel: "ground_check", Args: map[string]datalog.Term{
				"run_id": datalog.Val("literal"),
			}},
		},
	}

	assert.True(t, HeadExposesRunID(rules, "scoped_check"))
	assert.False(t, HeadExposesRunID(rules, "global_check"))
	assert.False(t, HeadExposesRunID(rules, "ground_check"),
		"a ground head arg is not a binding point; constraining it would be a no-op or a contradiction")
	assert.False(t, HeadExposesRunID(rules, "relation_with_no_rule"))
}
