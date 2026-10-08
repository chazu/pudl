// Package fieldpath parses and evaluates the field paths schemas use to name a
// location inside a record: identity fields, fact projections, sensitive
// fields and `pudl import --set` keys all share this one syntax.
//
//	path     := segment ("." segment)*
//	segment  := (ident | quoted) ("[*]")*
//	ident    := any run of characters other than '.'
//	quoted   := '"' JSON string body '"'     (may contain dots)
//
// Examples: name, metadata.name, metadata.labels."cloud.googleapis.com/location",
// sourceRanges[*], allowed[*].IPProtocol.
//
// Every dot-separated path that parsed before quoting existed parses to the
// same keys, so stored identity fields keep their meaning. The package is pure:
// no database or importer dependencies.
package fieldpath

import (
	"encoding/json"
	"fmt"
	"strings"
)

// step is one hop through a record: a map key, or a fan-out over an array.
type step struct {
	key      string
	wildcard bool
}

// Path is a parsed field path.
type Path struct {
	raw   string
	steps []step
}

// Parse parses a field path.
func Parse(s string) (Path, error) {
	if s == "" {
		return Path{}, fmt.Errorf("empty field path")
	}
	var steps []step
	rest := s
	for {
		var key string
		if strings.HasPrefix(rest, `"`) {
			end, err := quotedEnd(rest)
			if err != nil {
				return Path{}, fmt.Errorf("field path %q: %w", s, err)
			}
			if err := json.Unmarshal([]byte(rest[:end]), &key); err != nil {
				return Path{}, fmt.Errorf("field path %q: invalid quoted segment: %w", s, err)
			}
			rest = rest[end:]
			if key == "" {
				return Path{}, fmt.Errorf("field path %q: empty quoted segment", s)
			}
			steps = append(steps, step{key: key})
			for strings.HasPrefix(rest, "[*]") {
				steps = append(steps, step{wildcard: true})
				rest = rest[3:]
			}
		} else {
			seg := rest
			if i := strings.IndexByte(rest, '.'); i >= 0 {
				seg = rest[:i]
			}
			rest = rest[len(seg):]
			wildcards := 0
			for strings.HasSuffix(seg, "[*]") {
				seg = strings.TrimSuffix(seg, "[*]")
				wildcards++
			}
			if seg != "" {
				steps = append(steps, step{key: seg})
			} else if wildcards == 0 {
				return Path{}, fmt.Errorf("field path %q: empty segment", s)
			}
			for ; wildcards > 0; wildcards-- {
				steps = append(steps, step{wildcard: true})
			}
		}
		if rest == "" {
			break
		}
		if rest[0] != '.' {
			return Path{}, fmt.Errorf("field path %q: expected '.' after segment", s)
		}
		rest = rest[1:]
		if rest == "" {
			return Path{}, fmt.Errorf("field path %q: trailing '.'", s)
		}
	}
	return Path{raw: s, steps: steps}, nil
}

// MustParse parses a path known to be valid; it panics otherwise.
func MustParse(s string) Path {
	p, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return p
}

// quotedEnd returns the index just past the closing quote of the JSON string
// that starts s.
func quotedEnd(s string) (int, error) {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated quoted segment")
}

// String returns the path as written.
func (p Path) String() string { return p.raw }

// HasWildcard reports whether the path fans out over an array.
func (p Path) HasWildcard() bool {
	for _, s := range p.steps {
		if s.wildcard {
			return true
		}
	}
	return false
}

// Lookup returns the value at the path. It reports false when any step is
// absent or the path contains a wildcard.
func (p Path) Lookup(v any) (any, bool) {
	if p.HasWildcard() {
		return nil, false
	}
	cur := v
	for _, s := range p.steps {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[s.key]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// LookupAll returns every value the path reaches, expanding each wildcard over
// the elements of an array. Absent steps contribute nothing.
func (p Path) LookupAll(v any) []any {
	cur := []any{v}
	for _, s := range p.steps {
		var next []any
		for _, c := range cur {
			if s.wildcard {
				if arr, ok := c.([]any); ok {
					next = append(next, arr...)
				}
				continue
			}
			if m, ok := c.(map[string]any); ok {
				if val, ok := m[s.key]; ok {
					next = append(next, val)
				}
			}
		}
		cur = next
		if len(cur) == 0 {
			return nil
		}
	}
	return cur
}

// Set stores val at the path in m, creating intermediate objects as needed.
// Wildcard paths, and paths that cross a non-object value, are errors.
func (p Path) Set(m map[string]any, val any) error {
	if p.HasWildcard() {
		return fmt.Errorf("field path %q: cannot set through a wildcard", p.raw)
	}
	cur := m
	for i, s := range p.steps {
		if i == len(p.steps)-1 {
			cur[s.key] = val
			return nil
		}
		next, exists := cur[s.key]
		if !exists {
			child := map[string]any{}
			cur[s.key] = child
			cur = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("field path %q: %q is not an object", p.raw, s.key)
		}
		cur = child
	}
	return nil
}

// Replace rewrites every value the path reaches with fn(value) and returns how
// many were rewritten. Wildcards expand over arrays; absent steps are skipped.
func (p Path) Replace(v any, fn func(any) any) int {
	return replace(v, p.steps, fn)
}

func replace(v any, steps []step, fn func(any) any) int {
	if len(steps) == 0 {
		return 0
	}
	s, rest := steps[0], steps[1:]
	if s.wildcard {
		arr, ok := v.([]any)
		if !ok {
			return 0
		}
		n := 0
		for i := range arr {
			if len(rest) == 0 {
				arr[i] = fn(arr[i])
				n++
				continue
			}
			n += replace(arr[i], rest, fn)
		}
		return n
	}
	m, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	val, ok := m[s.key]
	if !ok {
		return 0
	}
	if len(rest) == 0 {
		m[s.key] = fn(val)
		return 1
	}
	return replace(val, rest, fn)
}

// FromKeys builds a path from literal keys, as if each were quoted. It is how
// a nested CUE struct (`set: metadata: labels: env: "x"`) becomes leaf paths.
func FromKeys(keys ...string) Path {
	steps := make([]step, len(keys))
	parts := make([]string, len(keys))
	for i, k := range keys {
		steps[i] = step{key: k}
		parts[i] = k
		if strings.ContainsAny(k, `."[`) || k == "" {
			b, _ := json.Marshal(k)
			parts[i] = string(b)
		}
	}
	return Path{raw: strings.Join(parts, "."), steps: steps}
}

// Overlaps reports whether one path is a prefix of the other, treating
// wildcards as matching only wildcards: redacting either location rewrites
// (part of) the other.
func (p Path) Overlaps(other Path) bool {
	n := min(len(p.steps), len(other.steps))
	for i := 0; i < n; i++ {
		if p.steps[i] != other.steps[i] {
			return false
		}
	}
	return true
}
