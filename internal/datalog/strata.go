package datalog

import (
	"fmt"
	"sort"
	"strings"
)

// A component is one strongly connected component of the derived-relation
// dependency graph: a set of relations that (transitively) read each other.
// A component is cyclic when it has more than one relation or a relation that
// reads itself; only cyclic components need fixpoint iteration.
type component struct {
	rels   []string
	cyclic bool
}

// queryPlan is the goal-directed evaluation plan for one relation: the derived
// relations it transitively reads, grouped into components in dependency order
// (every component appears after the components it reads).
type queryPlan struct {
	relation   string
	components []component
	byHead     map[string][]Rule
	headCols   map[string][]string
}

// hasCycle reports whether any component in the plan needs fixpoint iteration.
func (p *queryPlan) hasCycle() bool {
	for _, c := range p.components {
		if c.cyclic {
			return true
		}
	}
	return false
}

// derived reports whether the plan's relation is produced by rules (as opposed
// to being read directly from stored facts).
func (p *queryPlan) derived() bool {
	return len(p.byHead[p.relation]) > 0
}

// planQuery builds the evaluation plan for relation from valid rules. Only the
// relation's dependency closure is planned, so an unrelated relation — however
// deep its recursion — is never evaluated. Aggregation is allowed in any
// acyclic component; inside a cycle it has no stratified meaning and is
// rejected.
func planQuery(rules []Rule, relation string) (*queryPlan, error) {
	closure := dependencyClosure(rules, relation)
	plan := &queryPlan{
		relation: relation,
		byHead:   make(map[string][]Rule),
		headCols: make(map[string][]string),
	}
	for _, r := range rules {
		if !closure[r.Head.Rel] {
			continue
		}
		plan.byHead[r.Head.Rel] = append(plan.byHead[r.Head.Rel], r)
		if _, ok := plan.headCols[r.Head.Rel]; !ok {
			plan.headCols[r.Head.Rel] = sortedArgKeys(r.Head.Args)
		}
	}

	plan.components = stronglyConnected(plan.byHead)
	for _, c := range plan.components {
		if !c.cyclic {
			continue
		}
		for _, rel := range c.rels {
			for _, r := range plan.byHead[rel] {
				if ruleAggregates(r) {
					return nil, fmt.Errorf("relation %q aggregates inside a recursive cycle (%s), which is not supported",
						rel, strings.Join(c.rels, ", "))
				}
			}
		}
	}
	return plan, nil
}

// stronglyConnected returns the components of the derived-relation graph in
// dependency order, using Tarjan's algorithm. Tarjan emits a component only
// after every component reachable from it, which is exactly dependency order
// for edges that point from a rule head to the relations its body reads.
func stronglyConnected(byHead map[string][]Rule) []component {
	nodes := make([]string, 0, len(byHead))
	for rel := range byHead {
		nodes = append(nodes, rel)
	}
	sort.Strings(nodes) // deterministic order

	index := make(map[string]int)
	low := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	var out []component
	next := 0

	var visit func(rel string)
	visit = func(rel string) {
		index[rel], low[rel] = next, next
		next++
		stack = append(stack, rel)
		onStack[rel] = true

		for _, dep := range bodyDerived(byHead, rel) {
			if _, seen := index[dep]; !seen {
				visit(dep)
				low[rel] = min(low[rel], low[dep])
			} else if onStack[dep] {
				low[rel] = min(low[rel], index[dep])
			}
		}

		if low[rel] != index[rel] {
			return
		}
		var c component
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			c.rels = append(c.rels, top)
			if top == rel {
				break
			}
		}
		sort.Strings(c.rels)
		c.cyclic = len(c.rels) > 1 || readsItself(byHead, rel)
		out = append(out, c)
	}

	for _, rel := range nodes {
		if _, seen := index[rel]; !seen {
			visit(rel)
		}
	}
	return out
}

// bodyDerived lists the derived relations read by rel's rules, sorted.
func bodyDerived(byHead map[string][]Rule, rel string) []string {
	seen := make(map[string]bool)
	for _, r := range byHead[rel] {
		for _, a := range r.Body {
			if _, derived := byHead[a.Rel]; derived {
				seen[a.Rel] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for dep := range seen {
		out = append(out, dep)
	}
	sort.Strings(out)
	return out
}

func readsItself(byHead map[string][]Rule, rel string) bool {
	for _, r := range byHead[rel] {
		for _, a := range r.Body {
			if a.Rel == rel {
				return true
			}
		}
	}
	return false
}

func ruleAggregates(r Rule) bool {
	for _, t := range r.Head.Args {
		if t.IsAggregate() {
			return true
		}
	}
	return false
}
