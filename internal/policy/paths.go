package policy

import (
	"strconv"
	"strings"

	"github.com/chazu/pudl/internal/fieldpath"
)

// pathString renders tree segments in internal/fieldpath syntax: keys joined
// by '.', quoted when needed, and list elements as a [*] suffix.
func pathString(segs []string) string {
	var b strings.Builder
	for _, s := range segs {
		if s == wildcard {
			b.WriteString(wildcard)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(fieldpath.FromKeys(s).String())
	}
	return b.String()
}

// pathOf parses tree segments as a fieldpath.Path.
func pathOf(segs []string) (fieldpath.Path, error) {
	return fieldpath.Parse(pathString(segs))
}

// errorSegment is one element of a CUE error path.
type errorSegment struct {
	key   string
	index int // >= 0 for a list index
	// meta: a hidden field or definition, never record data.
	meta bool
}

// parseErrorPath converts the label strings of a CUE error path (as returned
// by errors.Error.Path) into segments. CUE prints identifiers bare, other
// string labels JSON-quoted, list indices as digits, and hidden fields and
// definitions with their _ or # prefix.
func parseErrorPath(labels []string) []errorSegment {
	out := make([]errorSegment, 0, len(labels))
	for _, l := range labels {
		switch {
		case strings.HasPrefix(l, `"`):
			key, err := strconv.Unquote(l)
			if err != nil {
				key = strings.Trim(l, `"`)
			}
			out = append(out, errorSegment{key: key, index: -1})
		case strings.HasPrefix(l, "_") || strings.HasPrefix(l, "#"):
			out = append(out, errorSegment{key: l, index: -1, meta: true})
		default:
			if i, err := strconv.Atoi(l); err == nil && i >= 0 {
				out = append(out, errorSegment{index: i})
			} else {
				out = append(out, errorSegment{key: l, index: -1})
			}
		}
	}
	return out
}

// resolveInRecord follows segments through record. It reports the canonical
// tree segments (list indices become [*]) and whether every step is present.
func resolveInRecord(record any, segs []errorSegment) ([]string, bool) {
	canon := make([]string, 0, len(segs))
	cur := record
	for _, s := range segs {
		if s.meta {
			return nil, false
		}
		if s.index >= 0 {
			list, ok := cur.([]any)
			if !ok || s.index >= len(list) {
				return nil, false
			}
			cur = list[s.index]
			canon = append(canon, wildcard)
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := obj[s.key]
		if !ok {
			return nil, false
		}
		cur = next
		canon = append(canon, s.key)
	}
	return canon, true
}
