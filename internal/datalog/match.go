package datalog

import (
	"encoding/json"
	"fmt"
)

// matchConstraints checks if a tuple satisfies the given field constraints.
func matchConstraints(t Tuple, constraints map[string]interface{}) bool {
	for k, v := range constraints {
		actual, has := t.Args[k]
		if !has || !valuesEqual(actual, v) {
			return false
		}
	}
	return true
}

// valuesEqual compares two values for equality, handling numeric type coercion.
func valuesEqual(a, b interface{}) bool {
	// Try direct comparison
	if fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) {
		return true
	}
	// Numeric coercion: JSON numbers are float64, Go literals might be int
	af, aOk := toFloat64(a)
	bf, bOk := toFloat64(b)
	if aOk && bOk {
		return af == bf
	}
	return false
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
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}
