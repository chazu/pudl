package scrub

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/redact"
)

// minSearchBytes is the shortest collected value searched for in unstructured
// artifacts. Shorter values ("true", "admin") would redact unrelated text.
const minSearchBytes = 8

// valueSet holds the plaintext found at sensitive paths during one run. It
// lives in memory only and is never printed, logged or stored.
type valueSet struct {
	values []string // at least minSearchBytes long, longest first once sealed
	seen   map[string]bool
	short  map[string]bool
	sealed bool
}

func newValueSet() *valueSet {
	return &valueSet{seen: map[string]bool{}, short: map[string]bool{}}
}

// collect adds every leaf string and number under x.
func (v *valueSet) collect(x any) {
	switch t := x.(type) {
	case string:
		v.add(t)
	case json.Number:
		v.add(t.String())
	case map[string]any:
		for _, child := range t {
			v.collect(child)
		}
	case []any:
		for _, child := range t {
			v.collect(child)
		}
	}
}

func (v *valueSet) add(s string) {
	if s == "" || s == redact.Placeholder || v.seen[s] || v.short[s] {
		return
	}
	if len(s) < minSearchBytes {
		v.short[s] = true
		return
	}
	v.seen[s] = true
	v.values = append(v.values, s)
	v.sealed = false
}

func (v *valueSet) empty() bool { return len(v.values) == 0 }

// skipped counts the distinct values too short to search for.
func (v *valueSet) skipped() int { return len(v.short) }

func (v *valueSet) seal() {
	if !v.sealed {
		// Longest first, so a value that contains another is replaced whole.
		sort.Slice(v.values, func(i, j int) bool { return len(v.values[i]) > len(v.values[j]) })
		v.sealed = true
	}
}

// mentionedIn is a fast pre-filter over raw JSON bytes: it matches each value
// as written and as JSON escapes it. A match is confirmed by replace.
func (v *valueSet) mentionedIn(raw []byte) bool {
	for _, s := range v.values {
		if bytes.Contains(raw, []byte(s)) {
			return true
		}
		if escaped, err := json.Marshal(s); err == nil && bytes.Contains(raw, escaped[1:len(escaped)-1]) {
			return true
		}
	}
	return false
}

// replace substitutes redact.Placeholder for every occurrence of a value in
// the strings under x (and for numbers equal to one), returning the new value
// and how many strings changed. Maps and slices are changed in place.
func (v *valueSet) replace(x any) (any, int) {
	v.seal()
	switch t := x.(type) {
	case string:
		out := t
		for _, s := range v.values {
			out = strings.ReplaceAll(out, s, redact.Placeholder)
		}
		if out != t {
			return out, 1
		}
	case json.Number:
		if v.seen[t.String()] {
			return redact.Placeholder, 1
		}
	case map[string]any:
		n := 0
		for k, child := range t {
			var c int
			t[k], c = v.replace(child)
			n += c
		}
		return t, n
	case []any:
		n := 0
		for i, child := range t {
			var c int
			t[i], c = v.replace(child)
			n += c
		}
		return t, n
	}
	return x, 0
}

// rewriteJSON replaces the values inside a stored JSON document, reporting
// whether anything changed. Numbers keep their exact digits.
func (v *valueSet) rewriteJSON(raw []byte) ([]byte, bool, error) {
	decoded, err := idgen.DecodeJSONExact(raw)
	if err != nil {
		return nil, false, err
	}
	replaced, n := v.replace(decoded)
	if n == 0 {
		return raw, false, nil
	}
	out, err := json.Marshal(replaced)
	return out, err == nil, err
}
