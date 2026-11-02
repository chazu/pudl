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
		next, seg, ok := step(cur, l)
		if !ok {
			return nil, false
		}
		cur = next
		canon = append(canon, seg)
	}
	return canon, true
}

// removeRecordPath deletes the object field that labels (as in recordPath)
// lead to, reporting whether it was there.
func removeRecordPath(record any, labels []string) bool {
	if len(labels) == 0 {
		return false
	}
	cur := record
	for _, l := range labels[:len(labels)-1] {
		next, _, ok := step(cur, l)
		if !ok {
			return false
		}
		cur = next
	}
	last := labels[len(labels)-1]
	if _, seg, ok := step(cur, last); !ok || seg == wildcard {
		return false
	}
	obj := cur.(map[string]any)
	delete(obj, errorLabelKey(last))
	return true
}

// step follows one error-path label from cur. It returns the value reached
// and its canonical tree segment.
func step(cur any, l string) (any, string, bool) {
	if strings.HasPrefix(l, "_") || strings.HasPrefix(l, "#") {
		return nil, "", false
	}
	if !strings.HasPrefix(l, `"`) {
		if i, err := strconv.Atoi(l); err == nil && i >= 0 {
			list, ok := cur.([]any)
			if !ok || i >= len(list) {
				return nil, "", false
			}
			return list[i], wildcard, true
		}
	}
	obj, ok := cur.(map[string]any)
	if !ok {
		return nil, "", false
	}
	key := errorLabelKey(l)
	next, ok := obj[key]
	if !ok {
		return nil, "", false
	}
	return next, key, true
}

// errorLabelKey decodes a JSON-quoted error-path label; identifiers are bare.
func errorLabelKey(l string) string {
	if !strings.HasPrefix(l, `"`) {
		return l
	}
	if key, err := strconv.Unquote(l); err == nil {
		return key
	}
	return strings.Trim(l, `"`)
}
