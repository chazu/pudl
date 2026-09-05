package datalog

import (
	"math/big"
	"reflect"
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

// valuesEqual compares normalized query values. Mixed INTEGER/REAL comparison
// uses exact binary values, as SQLite does, never converting the integer to REAL.
func valuesEqual(a, b interface{}) bool {
	af, bf := numericRat(a), numericRat(b)
	if af != nil && bf != nil {
		return af.Cmp(bf) == 0
	}
	return reflect.DeepEqual(a, b)
}

func numericRat(v interface{}) *big.Rat {
	switch n := v.(type) {
	case float64:
		return new(big.Rat).SetFloat64(n)
	case int64:
		return new(big.Rat).SetInt64(n)
	}
	return nil
}
