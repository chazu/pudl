package datalog

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Term is either a ground value or a variable reference. A variable may carry an
// aggregate function (Agg) when it appears in a rule head — e.g. count($S) yields
// Term{Variable:"$S", Agg:"count"}. Aggregates are head-only; the compiler rejects
// them in rule bodies.
type Term struct {
	Variable string      // non-empty if this is a variable (e.g. "$X")
	Value    interface{} // non-nil if this is a ground value
	Agg      string      // aggregate function over Variable: "count"|"sum"|"min"|"max" (head only)
}

// IsVariable returns true if the term is a variable.
func (t Term) IsVariable() bool {
	return t.Variable != ""
}

// IsAggregate returns true if the term applies an aggregate function.
func (t Term) IsAggregate() bool {
	return t.Agg != ""
}

// String returns a human-readable representation.
func (t Term) String() string {
	if t.IsVariable() {
		return t.Variable
	}
	return fmt.Sprintf("%v", t.Value)
}

// Var creates a variable term.
func Var(name string) Term {
	if !strings.HasPrefix(name, "$") {
		name = "$" + name
	}
	return Term{Variable: name}
}

// Val creates a ground value term.
func Val(v interface{}) Term {
	return Term{Value: v}
}

// Atom is a relation pattern with named arguments.
type Atom struct {
	Rel  string
	Args map[string]Term
}

// Rule defines a Datalog inference rule: head :- body.
type Rule struct {
	Name string
	Head Atom
	Body []Atom
}

// Tuple is a ground fact — a relation with concrete argument values.
type Tuple struct {
	Relation string
	Args     map[string]interface{}
}

// Key returns a deterministic string key for deduplication.
func (t Tuple) Key() string {
	h := sha256.New()
	h.Write([]byte(t.Relation))
	h.Write([]byte{0})

	// Sort keys for determinism
	keys := make([]string, 0, len(t.Args))
	for k := range t.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		v, _ := json.Marshal(t.Args[k])
		h.Write(v)
		h.Write([]byte{0})
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// aggTermPattern matches an aggregate applied to a variable, e.g. "count($S)".
var aggTermPattern = regexp.MustCompile(`^(count|sum|min|max)\((\$[A-Za-z_][A-Za-z0-9_]*)\)$`)

// ParseTerm converts a raw value (from CUE or JSON) into a Term.
// Strings starting with "$" are treated as variables; strings of the form
// "agg($Var)" (count/sum/min/max) become aggregate variable terms.
func ParseTerm(v interface{}) Term {
	if s, ok := v.(string); ok {
		if m := aggTermPattern.FindStringSubmatch(s); m != nil {
			return Term{Variable: m[2], Agg: m[1]}
		}
		if strings.HasPrefix(s, "$") {
			return Term{Variable: s}
		}
	}
	return Term{Value: v}
}
