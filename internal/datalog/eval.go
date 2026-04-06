package datalog

import (
	"encoding/json"
	"fmt"
)

// Evaluator performs bottom-up Datalog evaluation using the semi-naive algorithm.
type Evaluator struct {
	rules    []Rule
	edb      EDB
	maxIter  int // safety limit on iterations (default 100)
}

// NewEvaluator creates a Datalog evaluator with the given rules and EDB.
func NewEvaluator(rules []Rule, edb EDB) *Evaluator {
	return &Evaluator{
		rules:   rules,
		edb:     edb,
		maxIter: 100,
	}
}

// SetMaxIterations sets the maximum number of fixed-point iterations.
func (e *Evaluator) SetMaxIterations(n int) {
	e.maxIter = n
}

// Evaluate runs rules to fixed point, returning all derived facts (IDB).
func (e *Evaluator) Evaluate() ([]Tuple, error) {
	// known holds all facts (EDB + IDB), indexed by relation
	known := make(map[string]map[string]Tuple)
	// delta holds facts derived in the previous iteration
	delta := make(map[string]map[string]Tuple)

	// Seed known and delta with EDB facts for all relations mentioned in rules
	relations := e.mentionedRelations()
	for _, rel := range relations {
		tuples, err := e.edb.Scan(rel)
		if err != nil {
			return nil, fmt.Errorf("edb scan %s: %w", rel, err)
		}
		for _, t := range tuples {
			addTuple(known, t)
			addTuple(delta, t)
		}
	}

	// Semi-naive iteration
	for i := 0; i < e.maxIter; i++ {
		newFacts := make(map[string]map[string]Tuple)

		for _, rule := range e.rules {
			derived, err := e.fireRule(rule, known, delta)
			if err != nil {
				return nil, fmt.Errorf("rule %s: %w", rule.Name, err)
			}
			for _, t := range derived {
				key := t.Key()
				if _, exists := known[t.Relation][key]; !exists {
					addTuple(newFacts, t)
				}
			}
		}

		if len(newFacts) == 0 {
			break // fixed point reached
		}

		// Merge new facts into known, set as next delta
		delta = newFacts
		for rel, tuples := range newFacts {
			for key, t := range tuples {
				if known[rel] == nil {
					known[rel] = make(map[string]Tuple)
				}
				known[rel][key] = t
			}
		}
	}

	// Collect IDB (exclude EDB)
	edbKeys := make(map[string]bool)
	for _, rel := range relations {
		tuples, _ := e.edb.Scan(rel)
		for _, t := range tuples {
			edbKeys[t.Key()] = true
		}
	}

	var idb []Tuple
	for _, tuples := range known {
		for key, t := range tuples {
			if !edbKeys[key] {
				idb = append(idb, t)
			}
		}
	}
	return idb, nil
}

// Query evaluates to fixed point and returns derived facts matching the
// given relation and optional constraints.
func (e *Evaluator) Query(relation string, constraints map[string]interface{}) ([]Tuple, error) {
	all, err := e.Evaluate()
	if err != nil {
		return nil, err
	}

	// Also include matching EDB facts
	edbTuples, err := e.edb.Scan(relation)
	if err != nil {
		return nil, err
	}
	all = append(all, edbTuples...)

	var results []Tuple
	for _, t := range all {
		if t.Relation != relation {
			continue
		}
		if matchConstraints(t, constraints) {
			results = append(results, t)
		}
	}
	return results, nil
}

// fireRule finds all substitutions satisfying the rule body, requiring at
// least one body atom to match something in delta (semi-naive condition).
func (e *Evaluator) fireRule(rule Rule, known, delta map[string]map[string]Tuple) ([]Tuple, error) {
	var results []Tuple

	// For each "delta position" (which body atom uses delta facts),
	// compute all substitutions.
	for deltaIdx := range rule.Body {
		bindings := e.joinBody(rule.Body, known, delta, deltaIdx)

		for _, b := range bindings {
			t, err := b.Apply(rule.Head)
			if err != nil {
				continue // skip if head has unbound variables
			}
			results = append(results, t)
		}
	}

	return results, nil
}

// joinBody computes all consistent bindings for the rule body.
// The atom at deltaIdx must match facts from delta; others match from known.
func (e *Evaluator) joinBody(body []Atom, known, delta map[string]map[string]Tuple, deltaIdx int) []Binding {
	bindings := []Binding{{}}

	for i, atom := range body {
		var pool []Tuple
		if i == deltaIdx {
			pool = tuplesToSlice(delta[atom.Rel])
		} else {
			pool = tuplesToSlice(known[atom.Rel])
		}

		var next []Binding
		for _, b := range bindings {
			matches := matchAtom(atom, pool, b)
			next = append(next, matches...)
		}
		bindings = next

		if len(bindings) == 0 {
			break
		}
	}

	return bindings
}

// matchAtom finds all extensions of the given binding that satisfy the atom
// against the pool of tuples.
func matchAtom(atom Atom, pool []Tuple, existing Binding) []Binding {
	var results []Binding

	for _, t := range pool {
		if t.Relation != atom.Rel {
			continue
		}
		b := copyBinding(existing)
		ok := true

		for argKey, term := range atom.Args {
			factVal, has := t.Args[argKey]
			if !has {
				ok = false
				break
			}

			if term.IsVariable() {
				if prev, bound := b[term.Variable]; bound {
					if !valuesEqual(prev, factVal) {
						ok = false
						break
					}
				} else {
					b[term.Variable] = factVal
				}
			} else {
				if !valuesEqual(term.Value, factVal) {
					ok = false
					break
				}
			}
		}

		if ok {
			results = append(results, b)
		}
	}

	return results
}

// matchConstraints checks if a tuple satisfies the given field constraints.
func matchConstraints(t Tuple, constraints map[string]interface{}) bool {
	for k, v := range constraints {
		actual, has := t.Args[k]
		if !has || !valuesEqual(actual, v) {
			return false
		}
	}
	return true
}

// valuesEqual compares two values for equality, handling numeric type coercion.
func valuesEqual(a, b interface{}) bool {
	// Try direct comparison
	if fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) {
		return true
	}
	// Numeric coercion: JSON numbers are float64, Go literals might be int
	af, aOk := toFloat64(a)
	bf, bOk := toFloat64(b)
	if aOk && bOk {
		return af == bf
	}
	return false
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// mentionedRelations returns all unique relation names from rule bodies and heads.
func (e *Evaluator) mentionedRelations() []string {
	seen := make(map[string]bool)
	for _, r := range e.rules {
		seen[r.Head.Rel] = true
		for _, a := range r.Body {
			seen[a.Rel] = true
		}
	}
	rels := make([]string, 0, len(seen))
	for r := range seen {
		rels = append(rels, r)
	}
	return rels
}

func addTuple(m map[string]map[string]Tuple, t Tuple) {
	if m[t.Relation] == nil {
		m[t.Relation] = make(map[string]Tuple)
	}
	m[t.Relation][t.Key()] = t
}

func tuplesToSlice(m map[string]Tuple) []Tuple {
	if m == nil {
		return nil
	}
	s := make([]Tuple, 0, len(m))
	for _, t := range m {
		s = append(s, t)
	}
	return s
}

func copyBinding(b Binding) Binding {
	c := make(Binding, len(b))
	for k, v := range b {
		c[k] = v
	}
	return c
}
