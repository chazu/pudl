package projection

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
)

// Lint warns about the ways a rule silently matches nothing, for the rules the
// given relations depend on: a body relation nothing produces (no rule, no
// facts block, no stored facts) and an arg a facts block does not declare. A
// check over such a rule finds no rows and, with `expect: empty`, passes.
//
// declared maps projected relations to their args (Registry.RelationArgs);
// stored holds relations with current facts.
func Lint(rules []datalog.Rule, roots []string, declared map[string]map[string]bool, stored map[string]bool) []string {
	heads := map[string][]datalog.Rule{}
	for _, r := range rules {
		if r.Valid() {
			heads[r.Head.Rel] = append(heads[r.Head.Rel], r)
		}
	}
	seen := map[string]bool{}
	var warnings []string
	add := func(format string, args ...any) {
		w := fmt.Sprintf(format, args...)
		if !seen["w:"+w] {
			seen["w:"+w] = true
			warnings = append(warnings, w)
		}
	}
	var visit func(rel string)
	visit = func(rel string) {
		if seen[rel] {
			return
		}
		seen[rel] = true
		if declared[rel] != nil && len(heads[rel]) > 0 {
			add("relation %s is both projected by a schema and a rule head; queries see only the rule's tuples", rel)
		}
		for _, r := range heads[rel] {
			for _, atom := range r.Body {
				switch {
				case database.IsReservedRelation(atom.Rel):
				case declared[atom.Rel] != nil:
					for arg := range atom.Args {
						if !declared[atom.Rel][arg] {
							add("rule %s: relation %s has no arg %q (projected args: %s)", r.Describe(), atom.Rel, arg, argList(declared[atom.Rel]))
						}
					}
				case len(heads[atom.Rel]) == 0 && !stored[atom.Rel]:
					add("rule %s: relation %s has no facts, no rule and no schema facts block, so it matches nothing", r.Describe(), atom.Rel)
				}
				visit(atom.Rel)
			}
		}
	}
	for _, root := range roots {
		visit(root)
	}
	sort.Strings(warnings)
	return warnings
}

func argList(args map[string]bool) string {
	names := make([]string, 0, len(args))
	for a := range args {
		names = append(names, a)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
