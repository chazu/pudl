package database

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// ComputeFactID produces a content-addressed ID for a fact.
// ID = SHA256(relation + "\x00" + canonical_args + "\x00" + valid_start + "\x00" + source)
func ComputeFactID(relation, args string, validStart int64, source string) string {
	canonical := canonicalizeJSON(args)
	payload := fmt.Sprintf("%s\x00%s\x00%d\x00%s", relation, canonical, validStart, source)
	hash := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", hash)
}

// canonicalizeJSON sorts object keys and normalizes number spellings without
// rounding through float64. Invalid/non-object input retains the legacy raw
// representation. Existing persisted IDs are never recomputed on open.
func canonicalizeJSON(raw string) string {
	if !json.Valid([]byte(raw)) {
		return raw
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var obj map[string]interface{}
	if err := decoder.Decode(&obj); err != nil {
		return raw
	}
	canonical, err := json.Marshal(normalizeJSONNumbers(obj))
	if err != nil {
		return raw
	}
	return string(canonical)
}

func normalizeJSONNumbers(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, item := range v {
			v[key] = normalizeJSONNumbers(item)
		}
	case []interface{}:
		for i, item := range v {
			v[i] = normalizeJSONNumbers(item)
		}
	case json.Number:
		return canonicalNumber(v)
	}
	return value
}

// canonicalNumber uses encoding/json's decimal/scientific formatting cutoffs,
// preserving old hashes for numbers that were not rounded. The coefficient and
// exponent stay decimal, so even large integers and exponents are lossless and
// normalization never allocates a buffer proportional to an exponent's value.
// n must be a valid JSON number (provided by the decoder above).
func canonicalNumber(n json.Number) json.Number {
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
