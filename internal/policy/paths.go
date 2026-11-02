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

// recordPath follows the label strings of a CUE error path (as returned by
// errors.Error.Path) through record. CUE prints identifiers bare, other string
// labels JSON-quoted, list indices as digits, and hidden fields and
// definitions (never record data) with their _ or # prefix. It reports the
// canonical tree segments (list indices become [*]) and whether every step is
// present.
func recordPath(record any, labels []string) ([]string, bool) {
	canon := make([]string, 0, len(labels))
	cur := record
	for _, l := range labels {
		key := l
		switch {
		case strings.HasPrefix(l, `"`):
			var err error
			if key, err = strconv.Unquote(l); err != nil {
				key = strings.Trim(l, `"`)
			}
		case strings.HasPrefix(l, "_") || strings.HasPrefix(l, "#"):
			return nil, false
		default:
			if i, err := strconv.Atoi(l); err == nil && i >= 0 {
				list, ok := cur.([]any)
				if !ok || i >= len(list) {
					return nil, false
				}
				cur = list[i]
				canon = append(canon, wildcard)
				continue
			}
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = obj[key]; !ok {
			return nil, false
		}
		canon = append(canon, key)
	}
	return canon, true
}
