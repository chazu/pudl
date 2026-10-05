package idgen

import (
	"encoding/json"
	"testing"
)

func TestCanonicalJSONMatchesFloat64PathForRoundTrippingValues(t *testing.T) {
	for _, raw := range []string{
		`{"b":1.0,"a":[1e3,-0,0.1,1e-7,1e20,1e21]}`,
		`[1.25,{"z":"<&>","y":null,"x":true}]`,
		`9007199254740992`,
		`"text"`,
	} {
		var value interface{}
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
		legacy, _ := json.Marshal(value)
		got, err := CanonicalJSON([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if string(got) != string(legacy) {
			t.Errorf("%s: got %s, legacy %s", raw, got, legacy)
		}
	}
}

func TestCanonicalJSONKeepsDigitsFloat64Loses(t *testing.T) {
	got, err := CanonicalJSON([]byte(`{"id":9007199254740993,"d":0.10000000000000000001}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"d":0.10000000000000000001,"id":9007199254740993}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDecodeJSONExactRejectsTrailingContent(t *testing.T) {
	if _, err := DecodeJSONExact([]byte(`{"a":1} {"b":2}`)); err == nil {
		t.Fatal("expected an error for two concatenated values")
	}
}
