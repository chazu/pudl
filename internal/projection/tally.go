package projection

import (
	"fmt"
	"sort"
)

// maxOmittedWarnings caps the omitted-value samples a tally keeps.
const maxOmittedWarnings = 5

// Tally accumulates projection outcomes across one import or observation so
// the summary can say what was projected and, above all, what was not: a
// declared relation that never yielded a fact, values left out, and specs that
// are broken. Each of these would otherwise make an `expect: empty` check pass
// silently.
type Tally struct {
	Facts   map[string]int    // facts per relation
	broken  map[string]string // schema -> error
	omitted []string
	more    int
}

// NewTally returns an empty tally.
func NewTally() *Tally {
	return &Tally{Facts: map[string]int{}, broken: map[string]string{}}
}

// Note records one record of schema and what projecting it produced.
func (t *Tally) Note(reg *Registry, schema string, res Result) {
	if t == nil {
		return
	}
	if spec := reg.For(schema); spec != nil && spec.Err != nil {
		t.broken[spec.Schema] = spec.Err.Error()
		return
	}
	for rel, n := range res.PerRelation {
		t.Facts[rel] += n
	}
	for _, o := range res.Omitted {
		if len(t.omitted) < maxOmittedWarnings {
			t.omitted = append(t.omitted, o)
		} else {
			t.more++
		}
	}
}

// Warnings describes everything the user should know.
func (t *Tally) Warnings() []string {
	if t == nil {
		return nil
	}
	var out []string
	var schemas []string
	for s := range t.broken {
		schemas = append(schemas, s)
	}
	sort.Strings(schemas)
	for _, s := range schemas {
		out = append(out, fmt.Sprintf("projection disabled for %s (existing facts left unchanged): %s", s, t.broken[s]))
	}
	var rels []string
	for rel, n := range t.Facts {
		if n == 0 {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	for _, rel := range rels {
		out = append(out, fmt.Sprintf("relation %s produced no facts; check its paths against the data", rel))
	}
	for _, o := range t.omitted {
		out = append(out, "value omitted from facts: "+o)
	}
	if t.more > 0 {
		out = append(out, fmt.Sprintf("%d more values omitted from facts", t.more))
	}
	return out
}
