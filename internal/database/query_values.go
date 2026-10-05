package database

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/chazu/pudl/internal/idgen"
)

// QueryNumber maps a JSON decimal into the supported SQLite numeric domain.
// Canonicalization happens before conversion: integer exponent spellings must
// not travel through REAL, and a decimal that would lose digits is rejected.
// Raw fact storage and content identity have no such numeric range restriction.
func QueryNumber(n json.Number) (interface{}, error) {
	if _, err := json.Marshal(n); err != nil || n == "" {
		return nil, fmt.Errorf("unsupported query number %q: invalid JSON number", n)
	}
	canonical := idgen.CanonicalNumber(n)
	if i, err := strconv.ParseInt(string(canonical), 10, 64); err == nil {
		return i, nil
	}
	// Canonical integer values in range are plain decimal. Positive scientific
	// exponents here start at 21, beyond int64 and fractional binary64 precision.
	if strings.Contains(string(canonical), ".") || strings.Contains(string(canonical), "e-") {
		if f, err := canonical.Float64(); err == nil {
			encoded, err := json.Marshal(f)
			if err == nil && idgen.CanonicalNumber(json.Number(encoded)) == canonical &&
				!strings.Contains(string(canonical), "e+") {
				return f, nil
			}
		}
	}
	return nil, fmt.Errorf("unsupported query number %q: require an int64 integer or a decimal that round-trips through float64", n)
}

// QueryParameter normalizes numeric API operands before database/sql can turn
// json.Number into TEXT or a Go integer into a lossy floating-point value.
func QueryParameter(value interface{}) (interface{}, error) {
	if n, ok := value.(json.Number); ok {
		return QueryNumber(n)
	}
	v, err := driver.DefaultParameterConverter.ConvertValue(value)
	if err != nil {
		return nil, fmt.Errorf("unsupported query number or parameter: %w", err)
	}
	if f, ok := v.(float64); ok {
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("unsupported query number: non-finite float64")
		}
		if math.Trunc(f) == f {
			if f < -0x1p63 || f >= 0x1p63 {
				return nil, fmt.Errorf("unsupported query number: integer outside int64 range")
			}
			// A Go float already denotes a binary value. Its shortest JSON
			// spelling can name a different integer; convert the actual value.
			return int64(f), nil
		}
		return f, nil
	}
	return v, nil
}

// QueryConstraints returns normalized operands without modifying the caller's map.
func QueryConstraints(constraints map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(constraints))
	for key, value := range constraints {
		v, err := QueryParameter(value)
		if err != nil {
			return nil, fmt.Errorf("constraint %s: %w", key, err)
		}
		result[key] = v
	}
	return result, nil
}

// DecodeQueryJSON checks numbers recursively and preserves integers, including
// nested values, instead of json.Unmarshal's implicit float64 conversion.
func DecodeQueryJSON(raw string) (interface{}, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return queryJSONValue(value)
}

func queryJSONValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case json.Number:
		return QueryNumber(v)
	case map[string]interface{}:
		for key, item := range v {
			normalized, err := queryJSONValue(item)
			if err != nil {
				return nil, fmt.Errorf("argument %s: %w", key, err)
			}
			v[key] = normalized
		}
	case []interface{}:
		for i, item := range v {
			normalized, err := queryJSONValue(item)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", i, err)
			}
			v[i] = normalized
		}
	}
	return value, nil
}
