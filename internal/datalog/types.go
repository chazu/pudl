package datalog

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Term is either a ground value or a variable reference.
type Term struct {
	Variable string      // non-empty if this is a variable (e.g. "$X")
	Value    interface{} // non-nil if this is a ground value
}

// IsVariable returns true if the term is a variable.
func (t Term) IsVariable() bool {
	return t.Variable != ""
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

// Binding maps variable names to ground values.
type Binding map[string]interface{}

// Apply substitutes variables in an atom using the binding, returning a tuple.
// Returns an error if any variable is unbound.
func (b Binding) Apply(a Atom) (Tuple, error) {
	args := make(map[string]interface{}, len(a.Args))
	for k, term := range a.Args {
		if term.IsVariable() {
			val, ok := b[term.Variable]
			if !ok {
				return Tuple{}, fmt.Errorf("unbound variable %s", term.Variable)
			}
			args[k] = val
		} else {
			args[k] = term.Value
		}
	}
	return Tuple{Relation: a.Rel, Args: args}, nil
}

// ParseTerm converts a raw value (from CUE or JSON) into a Term.
// Strings starting with "$" are treated as variables.
func ParseTerm(v interface{}) Term {
	if s, ok := v.(string); ok && strings.HasPrefix(s, "$") {
		return Term{Variable: s}
	}
	return Term{Value: v}
}
