package datalog

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Term is either a ground value or a variable reference. A variable may carry an
// aggregate function (Agg) when it appears in a rule head — e.g. count($S) yields
// Term{Variable:"$S", Agg:"count"}. Aggregates are head-only; the compiler rejects
// them in rule bodies.
type Term struct {
	Variable string      // non-empty if this is a variable (e.g. "$X")
	Value    interface{} // ground value, or the numeric bound when Cmp is set
	Agg      string      // aggregate function over Variable: "count"|"sum"|"min"|"max" (head only)
	Cmp      string      // comparison operator against Value: ">"|"<"|">="|"<="|"!=" (body only)
	// Ago, when set, makes Cmp compare a date (an RFC 3339 or other SQLite
	// date string) with the evaluation time minus Ago seconds: the as-of time
	// of a time-travel query, else the current time. "<now-90d" is
	// Term{Cmp: "<", Ago: 90 days}.
	Ago *int64
}

// IsVariable returns true if the term is a variable.
func (t Term) IsVariable() bool {
	return t.Variable != ""
}

// IsAggregate returns true if the term applies an aggregate function.
func (t Term) IsAggregate() bool {
	return t.Agg != ""
}

// IsComparison returns true if the term is a comparison constraint, numeric
// (e.g. ">0.25") or a date against now ("<now-90d"), as opposed to a plain
// equality ground value.
func (t Term) IsComparison() bool {
	return t.Cmp != ""
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
//
// A rule loaded from CUE that is malformed keeps its place in the rule set with
// LoadErr set, rather than being dropped: a silently missing rule makes every
// query that depends on it return nothing, which a model check reads as a pass.
// Evaluate refuses to answer a query whose dependency closure contains such a
// rule. Head.Rel is filled in when the head relation could still be read.
type Rule struct {
	Name string
	Head Atom
	Body []Atom

	// Source is the rule's CUE position ("file:line:col"), when known.
	Source string
	// LoadErr is non-nil when the rule could not be loaded as written.
	LoadErr error
}

// Valid reports whether the rule loaded without error.
func (r Rule) Valid() bool {
	return r.LoadErr == nil
}

// Describe names the rule for error messages: its name and source position.
func (r Rule) Describe() string {
	name := r.Name
	if name == "" {
		name = "(unnamed)"
	}
	if r.Source != "" {
		return fmt.Sprintf("%s (%s)", name, r.Source)
	}
	return name
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

// cmpTermPattern matches a numeric comparison constraint, e.g. ">0.25" or "<=10".
// Two-character operators are listed first so they win the alternation.
var cmpTermPattern = regexp.MustCompile(`^(>=|<=|!=|>|<)\s*(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)$`)

// nowTermPattern matches a date comparison with the evaluation time, e.g.
// "<now-90d", ">=now-12h" or "<now". Units: d(ays), h(ours), m(inutes).
var nowTermPattern = regexp.MustCompile(`^(>=|<=|>|<)\s*now(?:\s*-\s*(\d+)\s*([dhm]))?$`)

var nowUnitSeconds = map[string]int64{"d": 86400, "h": 3600, "m": 60}

// ParseTerm converts a raw value (from CUE or JSON) into a Term.
// Strings starting with "$" are variables; "agg($Var)" (count/sum/min/max) are
// aggregate variable terms; "OP number" (>, <, >=, <=, != with a numeric literal)
// are comparison constraints, and "OP now[-N(d|h|m)]" (>, <, >=, <=) compares a
// date with the evaluation time. Everything else is a ground equality value.
func ParseTerm(v interface{}) Term {
	if s, ok := v.(string); ok {
		if m := aggTermPattern.FindStringSubmatch(s); m != nil {
			return Term{Variable: m[2], Agg: m[1]}
		}
		if m := nowTermPattern.FindStringSubmatch(s); m != nil {
			var ago int64
			if m[2] != "" {
				n, err := strconv.ParseInt(m[2], 10, 64)
				if err != nil {
					return Term{Value: v} // out of range: a plain value
				}
				ago = n * nowUnitSeconds[m[3]]
			}
			return Term{Cmp: m[1], Value: s, Ago: &ago}
		}
		if m := cmpTermPattern.FindStringSubmatch(s); m != nil {
			return Term{Cmp: m[1], Value: json.Number(m[2])}
		}
		if strings.HasPrefix(s, "$") {
			return Term{Variable: s}
		}
	}
	return Term{Value: v}
}
