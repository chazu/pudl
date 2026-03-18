package drift

import (
	"fmt"
	"reflect"
	"sort"
	"time"
)

// DriftResult holds the outcome of comparing declared vs live state.
type DriftResult struct {
	Definition   string      `json:"definition"`
	Method       string      `json:"method"`
	Status       string      `json:"status"` // "clean", "drifted", "unknown"
	Timestamp    time.Time   `json:"timestamp"`
	DeclaredKeys map[string]interface{} `json:"declared_keys"`
	LiveState    map[string]interface{} `json:"live_state"`
	Differences  []FieldDiff `json:"differences"`
}

// FieldDiff describes a single field-level difference between declared and live state.
type FieldDiff struct {
	Path     string      `json:"path"`     // dot-notation path
	Type     string      `json:"type"`     // "changed", "added", "removed"
	Declared interface{} `json:"declared"` // nil for "added"
	Live     interface{} `json:"live"`     // nil for "removed"
}

// Compare performs a recursive deep diff between declared and live maps.
// "removed" means present in declared but missing from live.
// "added" means present in live but missing from declared.
// "changed" means present in both but with different values.
func Compare(declared, live map[string]interface{}) []FieldDiff {
	var diffs []FieldDiff
	compareRecursive("", declared, live, &diffs)
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Path < diffs[j].Path
	})
	return diffs
}

func compareRecursive(prefix string, declared, live map[string]interface{}, diffs *[]FieldDiff) {
	// Check all declared keys
	for key, declVal := range declared {
		path := joinPath(prefix, key)
		liveVal, exists := live[key]

		if !exists {
			*diffs = append(*diffs, FieldDiff{
				Path:     path,
				Type:     "removed",
				Declared: declVal,
				Live:     nil,
			})
			continue
		}

		// Both exist — recurse if both are maps, otherwise compare
		declMap, declIsMap := toMap(declVal)
		liveMap, liveIsMap := toMap(liveVal)

		if declIsMap && liveIsMap {
			compareRecursive(path, declMap, liveMap, diffs)
		} else if !valuesEqual(declVal, liveVal) {
			*diffs = append(*diffs, FieldDiff{
				Path:     path,
				Type:     "changed",
				Declared: declVal,
				Live:     liveVal,
			})
		}
	}

	// Check for keys in live not in declared
	for key, liveVal := range live {
		path := joinPath(prefix, key)
		if _, exists := declared[key]; !exists {
			*diffs = append(*diffs, FieldDiff{
				Path:     path,
				Type:     "added",
				Declared: nil,
				Live:     liveVal,
			})
		}
	}
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return fmt.Sprintf("%s.%s", prefix, key)
}

// toMap attempts to convert a value to map[string]interface{}.
func toMap(v interface{}) (map[string]interface{}, bool) {
	if v == nil {
		return nil, false
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m, true
	}
	return nil, false
}

// valuesEqual compares two values for equality, handling numeric type coercion.
func valuesEqual(a, b interface{}) bool {
	// Handle numeric comparisons across types (JSON numbers are float64)
	aNum, aIsNum := toFloat64(a)
	bNum, bIsNum := toFloat64(b)
	if aIsNum && bIsNum {
		return aNum == bNum
	}

	return reflect.DeepEqual(a, b)
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}
