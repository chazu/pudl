package acute

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
)

// CompareDesired returns every desired field the observed record does not
// satisfy, sorted by path.
//
// Top-level comparison is ensure-present: the observed record may carry fields
// the desired record does not mention. Below the top level values compare
// exactly — a nested map with an extra observed key drifts — which preserves the
// semantics of the whole-value comparison this replaces. Equality is typed:
// numbers compare by value whatever their Go representation, but a number never
// equals a string, so 1 and "1" differ.
func CompareDesired(desired, observed map[string]any) []FieldDiff {
	return compareDesired(desired, observed, false)
}

func compareDesired(desired, observed map[string]any, structural bool) []FieldDiff {
	var diffs []FieldDiff
	for _, k := range sortedKeys(desired) {
		if k == "_schema" {
			continue
		}
		ov, ok := observed[k]
		if !ok {
			diffs = append(diffs, fieldAt(FieldDiff{Path: k, Expected: desired[k], Missing: true}, []PathComponent{{Key: &k}}, structural))
			continue
		}
		diffs = compareValueAt(k, desired[k], ov, diffs, []PathComponent{{Key: &k}}, structural)
	}
	return diffs
}

// compareValue appends the differences between an expected and an observed
// value at path to diffs.
func compareValueAt(path string, expected, observed any, diffs []FieldDiff, components []PathComponent, structural bool) []FieldDiff {
	em, eIsMap := asMap(expected)
	om, oIsMap := asMap(observed)
	if eIsMap && oIsMap {
		for _, k := range unionKeys(em, om) {
			child := joinPath(path, k)
			childParts := append(append([]PathComponent{}, components...), PathComponent{Key: &k})
			ev, declared := em[k]
			ov, present := om[k]
			switch {
			case !present:
				diffs = append(diffs, fieldAt(FieldDiff{Path: child, Expected: ev, Missing: true}, childParts, structural))
			case !declared:
				diffs = append(diffs, fieldAt(FieldDiff{Path: child, Observed: ov, Unexpected: true}, childParts, structural))
			default:
				diffs = compareValueAt(child, ev, ov, diffs, childParts, structural)
			}
		}
		return diffs
	}

	el, eIsList := expected.([]any)
	ol, oIsList := observed.([]any)
	if eIsList && oIsList && len(el) == len(ol) {
		for i := range el {
			diffs = compareValueAt(fmt.Sprintf("%s[%d]", path, i), el[i], ol[i], diffs, append(append([]PathComponent{}, components...), PathComponent{Index: &i}), structural)
		}
		return diffs
	}

	if !ValuesEqual(expected, observed) {
		diffs = append(diffs, fieldAt(FieldDiff{Path: path, Expected: expected, Observed: observed}, components, structural))
	}
	return diffs
}

// ValuesEqual reports whether two JSON-like values are equal, comparing numbers
// by value regardless of their Go type and everything else by type and value.
func ValuesEqual(a, b any) bool {
	ra, aNum := numberValue(a)
	rb, bNum := numberValue(b)
	if aNum || bNum {
		return aNum && bNum && ra.Cmp(rb) == 0
	}
	am, aIsMap := asMap(a)
	bm, bIsMap := asMap(b)
	if aIsMap || bIsMap {
		if !aIsMap || !bIsMap || len(am) != len(bm) {
			return false
		}
		for k, av := range am {
			bv, ok := bm[k]
			if !ok || !ValuesEqual(av, bv) {
				return false
			}
		}
		return true
	}
	al, aIsList := a.([]any)
	bl, bIsList := b.([]any)
	if aIsList || bIsList {
		if !aIsList || !bIsList || len(al) != len(bl) {
			return false
		}
		for i := range al {
			if !ValuesEqual(al[i], bl[i]) {
				return false
			}
		}
		return true
	}
	switch av := a.(type) {
	case nil:
		return b == nil
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	}
	return fmt.Sprintf("%T:%v", a, a) == fmt.Sprintf("%T:%v", b, b)
}

// numberValue converts any numeric representation decoders produce (Go
// integers and floats, json.Number, big values) to an exact rational.
func numberValue(v any) (*big.Rat, bool) {
	switch n := v.(type) {
	case int:
		return new(big.Rat).SetInt64(int64(n)), true
	case int8:
		return new(big.Rat).SetInt64(int64(n)), true
	case int16:
		return new(big.Rat).SetInt64(int64(n)), true
	case int32:
		return new(big.Rat).SetInt64(int64(n)), true
	case int64:
		return new(big.Rat).SetInt64(n), true
	case uint:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint8:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint16:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint32:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint64:
		return new(big.Rat).SetUint64(n), true
	case float32:
		return floatRat(float64(n))
	case float64:
		return floatRat(n)
	case json.Number:
		r, ok := new(big.Rat).SetString(n.String())
		return r, ok
	case *big.Int:
		return new(big.Rat).SetInt(n), true
	case *big.Float:
		r, _ := n.Rat(nil)
		return r, r != nil
	}
	return nil, false
}

// floatRat converts a finite float; NaN and infinities are not numbers that can
// equal anything, so they are reported as non-numeric and compare by type.
func floatRat(f float64) (*big.Rat, bool) {
	r := new(big.Rat)
	if r.SetFloat64(f) == nil {
		return nil, false
	}
	return r, true
}

// FormatValue renders a value as compact JSON, falling back to Go formatting
// for values JSON cannot represent.
func FormatValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// unionKeys returns the keys of both maps, sorted and without duplicates.
func unionKeys(a, b map[string]any) []string {
	keys := sortedKeys(a)
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func fieldAt(diff FieldDiff, components []PathComponent, structural bool) FieldDiff {
	if structural {
		diff.PathComponents = components
	}
	return diff
}
