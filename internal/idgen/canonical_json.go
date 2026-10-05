package idgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// DecodeJSONExact decodes one JSON value without rounding numbers through
// float64. Numbers come back as json.Number in their canonical spelling (see
// CanonicalNumber), so a value that round-trips through float64 has the same
// spelling encoding/json would have produced for it, and one that does not
// (an integer beyond 2^53, a long decimal) keeps every digit.
func DecodeJSONExact(raw []byte) (interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if decoder.More() {
		return nil, fmt.Errorf("unexpected content after JSON value")
	}
	return NormalizeJSONNumbers(value), nil
}

// CanonicalJSON returns the canonical encoding of a JSON value: object keys
// sorted, numbers in canonical spelling, no insignificant whitespace. For a
// value whose numbers all survive a float64 round trip this is byte-identical
// to json.Marshal of the float64-decoded value, which keeps content hashes
// computed by the earlier float64 path stable.
func CanonicalJSON(raw []byte) ([]byte, error) {
	value, err := DecodeJSONExact(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// NormalizeJSONNumbers rewrites every json.Number inside value to its canonical
// spelling, in place for maps and slices, and returns the result.
func NormalizeJSONNumbers(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, item := range v {
			v[key] = NormalizeJSONNumbers(item)
		}
	case []interface{}:
		for i, item := range v {
			v[i] = NormalizeJSONNumbers(item)
		}
	case json.Number:
		return CanonicalNumber(v)
	}
	return value
}

// CanonicalNumber uses encoding/json's decimal/scientific formatting cutoffs,
// preserving old hashes for numbers that were not rounded. The coefficient and
// exponent stay decimal, so even large integers and exponents are lossless and
// normalization never allocates a buffer proportional to an exponent's value.
// n must be a valid JSON number (as produced by a json.Decoder).
func CanonicalNumber(n json.Number) json.Number {
	text, negative := strings.CutPrefix(string(n), "-")
	sign := ""
	if negative {
		sign = "-"
	}
	coefficient, exponent, _ := strings.Cut(strings.ToLower(text), "e")
	var power big.Int
	if exponent != "" {
		power.SetString(exponent, 10)
	}
	if dot := strings.IndexByte(coefficient, '.'); dot >= 0 {
		power.Sub(&power, big.NewInt(int64(len(coefficient)-dot-1)))
		coefficient = coefficient[:dot] + coefficient[dot+1:]
	}
	coefficient = strings.TrimLeft(coefficient, "0")
	if coefficient == "" {
		return json.Number(sign + "0")
	}
	digits := strings.TrimRight(coefficient, "0")
	// power becomes the exponent of the first significant digit.
	power.Add(&power, big.NewInt(int64(len(coefficient)-1)))
	if power.IsInt64() && power.Int64() >= -6 && power.Int64() < 21 {
		point := int(power.Int64()) + 1
		switch {
		case point <= 0:
			return json.Number(sign + "0." + strings.Repeat("0", -point) + digits)
		case point >= len(digits):
			return json.Number(sign + digits + strings.Repeat("0", point-len(digits)))
		default:
			return json.Number(sign + digits[:point] + "." + digits[point:])
		}
	}
	mantissa := digits[:1]
	if len(digits) > 1 {
		mantissa += "." + digits[1:]
	}
	exponent = power.String()
	if power.Sign() >= 0 {
		exponent = "+" + exponent
	}
	return json.Number(sign + mantissa + "e" + exponent)
}
