package idgen

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

// FuzzCanonicalJSON checks the canonical encoding that content hashes and fact
// IDs are computed from:
//   - it accepts exactly one valid JSON value;
//   - it is idempotent (canonicalizing canonical output changes nothing);
//   - re-spacing the input does not change the output;
//   - numbers keep their exact decimal value.
func FuzzCanonicalJSON(f *testing.F) {
	for _, seed := range []string{
		`{"b":1,"a":2}`,
		` { "a" : [ 1 , 2.50 , -0 , 1e3 ] } `,
		`{"id":9007199254740993}`,
		`{"n":1e400,"m":1E-400,"k":0.000001,"j":123456789012345678901234567890}`,
		`[1.0,10e99999999999999999999,-1.25e-7]`,
		`"é<"`,
		`null`,
		`{"a":{"a":{"a":[]}}}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		canonical, err := CanonicalJSON(data)
		if !json.Valid(data) {
			if err == nil {
				t.Fatalf("canonicalized invalid JSON %q", data)
			}
			return
		}
		if err != nil {
			t.Fatalf("rejected valid JSON %q: %v", data, err)
		}
		again, err := CanonicalJSON(canonical)
		if err != nil || !bytes.Equal(again, canonical) {
			t.Fatalf("not idempotent: %q -> %q -> %q (%v)", data, canonical, again, err)
		}
		var indented bytes.Buffer
		if err := json.Indent(&indented, data, "", "  "); err == nil {
			respaced, err := CanonicalJSON(indented.Bytes())
			if err != nil || !bytes.Equal(respaced, canonical) {
				t.Fatalf("whitespace changed canonical form: %q vs %q", respaced, canonical)
			}
		}
		assertSameNumbers(t, data, canonical)
	})
}

// assertSameNumbers decodes both documents with UseNumber and checks every
// number pair has the same exact rational value.
func assertSameNumbers(t *testing.T, original, canonical []byte) {
	t.Helper()
	decode := func(b []byte) any {
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		var v any
		if err := d.Decode(&v); err != nil {
			t.Fatalf("decode %q: %v", b, err)
		}
		return v
	}
	var walk func(a, b any)
	walk = func(a, b any) {
		switch av := a.(type) {
		case map[string]any:
			bv, ok := b.(map[string]any)
			if !ok || len(av) != len(bv) {
				t.Fatalf("object shape changed: %#v vs %#v", a, b)
			}
			for k, item := range av {
				walk(item, bv[k])
			}
		case []any:
			bv, ok := b.([]any)
			if !ok || len(av) != len(bv) {
				t.Fatalf("array shape changed: %#v vs %#v", a, b)
			}
			for i := range av {
				walk(av[i], bv[i])
			}
		case json.Number:
			bn, ok := b.(json.Number)
			if !ok {
				t.Fatalf("number became %#v", b)
			}
			if !sameRational(string(av), string(bn)) {
				t.Fatalf("number %s canonicalized to %s with a different value", av, bn)
			}
		}
	}
	walk(decode(original), decode(canonical))
}

// sameRational compares two JSON numbers exactly. Exponents too large to
// expand are compared by their normalized coefficient/exponent instead.
func sameRational(a, b string) bool {
	if smallExponent(a) && smallExponent(b) {
		ra, okA := new(big.Rat).SetString(a)
		rb, okB := new(big.Rat).SetString(b)
		if okA && okB {
			return ra.Cmp(rb) == 0
		}
	}
	return string(CanonicalNumber(json.Number(a))) == string(CanonicalNumber(json.Number(b)))
}

// smallExponent reports whether a JSON number's exponent is small enough for
// big.Rat to expand without exhausting memory.
func smallExponent(n string) bool {
	i := strings.IndexAny(n, "eE")
	if i < 0 {
		return true
	}
	exp, err := strconv.Atoi(n[i+1:])
	return err == nil && exp > -2000 && exp < 2000
}
