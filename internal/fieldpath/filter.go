package fieldpath

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// A filtered wildcard fans out over an array like [*], except over the
// elements it keeps out: objects whose field holds one of the listed strings.
//
//	filter := "[*!" field "=" JSON-array-of-strings "]"
//	field  := quoted | run of characters other than '=', ']', '.', '"'
//
// metadata.items[*!key=["enable-oslogin"]].value reaches every item's value
// except the item whose key is "enable-oslogin". Redaction uses it as an
// allowlist: the listed items keep their values, every other value is
// redacted, including items with no such field or a non-string one. An
// element that is not an object is never kept out.

const filterOpen = "[*!"

// sep delimits the canonical value set; values may not contain it.
const sep = "\x00"

// parseFilter parses the filter that s starts with and returns its step and
// length.
func parseFilter(s string) (step, int, error) {
	i := len(filterOpen)
	var field string
	if i < len(s) && s[i] == '"' {
		end, err := quotedEnd(s[i:])
		if err != nil {
			return step{}, 0, fmt.Errorf("filter field: %w", err)
		}
		if err := json.Unmarshal([]byte(s[i:i+end]), &field); err != nil {
			return step{}, 0, fmt.Errorf("filter field: %w", err)
		}
		i += end
	} else {
		j := strings.IndexAny(s[i:], `=].`)
		if j < 0 {
			return step{}, 0, fmt.Errorf("filter: expected '=' after the field")
		}
		field = s[i : i+j]
		i += j
	}
	if field == "" {
		return step{}, 0, fmt.Errorf("filter: empty field")
	}
	if i >= len(s) || s[i] != '=' {
		return step{}, 0, fmt.Errorf("filter: expected '=' after the field")
	}
	i++
	if i >= len(s) || s[i] != '[' {
		return step{}, 0, fmt.Errorf("filter: expected a JSON array of strings after '='")
	}
	dec := json.NewDecoder(strings.NewReader(s[i:]))
	var values []string
	if err := dec.Decode(&values); err != nil {
		return step{}, 0, fmt.Errorf("filter: values must be a JSON array of strings: %w", err)
	}
	i += int(dec.InputOffset())
	if i >= len(s) || s[i] != ']' {
		return step{}, 0, fmt.Errorf("filter: expected ']' after the values")
	}
	set, err := valueSet(values)
	if err != nil {
		return step{}, 0, err
	}
	return step{wildcard: true, keepField: field, keepSet: set}, i + 1, nil
}

// valueSet renders values canonically (sorted, unique, delimited) so equal
// filters compare equal.
func valueSet(values []string) (string, error) {
	seen := map[string]bool{}
	var uniq []string
	for _, v := range values {
		if strings.Contains(v, sep) {
			return "", fmt.Errorf("filter: value %q contains a NUL character", v)
		}
		if !seen[v] {
			seen[v] = true
			uniq = append(uniq, v)
		}
	}
	if len(uniq) == 0 {
		return "", nil
	}
	sort.Strings(uniq)
	return sep + strings.Join(uniq, sep) + sep, nil
}

// keepsOut reports whether a filtered wildcard skips element v.
func (s step) keepsOut(v any) bool {
	if s.keepField == "" || s.keepSet == "" {
		return false
	}
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	val, ok := m[s.keepField].(string)
	if !ok {
		return false
	}
	return strings.Contains(s.keepSet, sep+val+sep)
}

// parseWildcards parses s as a run of [*] and filters. ok is false when s is
// not entirely such a run; err reports a malformed filter.
func parseWildcards(s string) (steps []step, ok bool, err error) {
	for s != "" {
		switch {
		case strings.HasPrefix(s, "[*]"):
			steps = append(steps, step{wildcard: true})
			s = s[3:]
		case strings.HasPrefix(s, filterOpen):
			st, n, err := parseFilter(s)
			if err != nil {
				return nil, false, err
			}
			steps = append(steps, st)
			s = s[n:]
		default:
			return nil, false, nil
		}
	}
	return steps, true, nil
}

// segmentEnd returns the length of the unquoted segment rest starts with: up
// to the first '.' outside a filter.
func segmentEnd(rest string) (int, error) {
	i := 0
	for i < len(rest) {
		if strings.HasPrefix(rest[i:], filterOpen) {
			_, n, err := parseFilter(rest[i:])
			if err != nil {
				return 0, err
			}
			i += n
			continue
		}
		if rest[i] == '.' {
			break
		}
		i++
	}
	return i, nil
}

// ExceptWhere builds the path of every element of the array each names
// (each ends in [*]) except those whose field holds one of keep, followed by
// value within each element ("" for the element itself).
func ExceptWhere(each, field string, keep []string, value string) (Path, error) {
	if !strings.HasSuffix(each, "[*]") {
		return Path{}, fmt.Errorf("each %q must end in [*]", each)
	}
	if field == "" {
		return Path{}, fmt.Errorf("empty filter field")
	}
	f := field
	if strings.ContainsAny(field, `=]."[`) {
		b, _ := json.Marshal(field)
		f = string(b)
	}
	if keep == nil {
		keep = []string{}
	}
	vals, err := json.Marshal(keep)
	if err != nil {
		return Path{}, err
	}
	s := strings.TrimSuffix(each, "[*]") + filterOpen + f + "=" + string(vals) + "]"
	if value != "" {
		s += "." + value
	}
	return Parse(s)
}
