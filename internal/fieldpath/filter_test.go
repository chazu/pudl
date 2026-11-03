package fieldpath

import (
	"reflect"
	"testing"
)

func metadataRecord() map[string]any {
	return map[string]any{
		"metadata": map[string]any{
			"items": []any{
				map[string]any{"key": "enable-oslogin", "value": "TRUE"},
				map[string]any{"key": "startup-script", "value": "echo secret"},
				map[string]any{"key": "cluster.name", "value": "prod"},
				map[string]any{"value": "no key"},
				map[string]any{"key": 7, "value": "numeric key"},
				"not an object",
			},
		},
	}
}

func TestFilterLookupAllSkipsKeptElements(t *testing.T) {
	p := MustParse(`metadata.items[*!key=["enable-oslogin","cluster.name"]].value`)
	got := p.LookupAll(metadataRecord())
	want := []any{"echo secret", "no key", "numeric key"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LookupAll = %v, want %v", got, want)
	}
	if !p.HasWildcard() {
		t.Fatal("a filtered wildcard is a wildcard")
	}
}

func TestFilterReplaceRedactsAllButKept(t *testing.T) {
	rec := metadataRecord()
	p := MustParse(`metadata.items[*!key=["enable-oslogin","cluster.name"]].value`)
	if n := p.Replace(rec, func(any) any { return "X" }); n != 3 {
		t.Fatalf("Replace rewrote %d values, want 3", n)
	}
	items := rec["metadata"].(map[string]any)["items"].([]any)
	values := []any{}
	for _, it := range items[:5] {
		values = append(values, it.(map[string]any)["value"])
	}
	want := []any{"TRUE", "X", "prod", "X", "X"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %v, want %v", values, want)
	}
}

func TestFilterOnWholeElementRedactsNonObjects(t *testing.T) {
	rec := metadataRecord()
	p := MustParse(`metadata.items[*!key=["enable-oslogin"]]`)
	p.Replace(rec, func(any) any { return "X" })
	items := rec["metadata"].(map[string]any)["items"].([]any)
	if items[5] != "X" {
		t.Fatalf("a non-object element must not be kept out, got %v", items[5])
	}
	if _, ok := items[0].(map[string]any); !ok {
		t.Fatalf("the kept element must be untouched, got %v", items[0])
	}
}

func TestFilterEmptyListKeepsNothing(t *testing.T) {
	p := MustParse(`metadata.items[*!key=[]].value`)
	if got := len(p.LookupAll(metadataRecord())); got != 5 {
		t.Fatalf("LookupAll found %d values, want 5", got)
	}
}

func TestFilterOneOrManySingleValue(t *testing.T) {
	p := MustParse(`s[*!k=["keep"]].v`)
	kept := map[string]any{"s": map[string]any{"k": "keep", "v": 1}}
	other := map[string]any{"s": map[string]any{"k": "other", "v": 2}}
	if got := p.LookupAllOneOrMany(kept); got != nil {
		t.Fatalf("kept single value reached: %v", got)
	}
	if got := p.LookupAllOneOrMany(other); !reflect.DeepEqual(got, []any{2}) {
		t.Fatalf("LookupAllOneOrMany = %v, want [2]", got)
	}
}

func TestFilterEqualityIgnoresValueOrder(t *testing.T) {
	a := MustParse(`m.items[*!key=["b","a","a"]].value`)
	b := MustParse(`m.items[*!key=["a","b"]].value`)
	if !a.Overlaps(b) || !a.HasPrefix(b) {
		t.Fatal("filters with the same values must compare equal")
	}
	plain := MustParse(`m.items[*].value`)
	if a.HasPrefix(plain) || plain.HasPrefix(a) {
		t.Fatal("a filtered wildcard must not match a plain one")
	}
}

func TestFilterParseForms(t *testing.T) {
	for _, s := range []string{
		`[*!key=["a"]]`,
		`items[*!key=["a.b","c]d"]].value`,
		`"quoted.key"[*!key=["a"]].value`,
		`items[*!"odd=field"=["a"]].value`,
		`items[*][*!key=["a"]][*].x`,
	} {
		p, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if p.String() != s {
			t.Fatalf("String() = %q, want %q", p.String(), s)
		}
	}
}

func TestFilterParseErrors(t *testing.T) {
	for _, s := range []string{
		`items[*!key=["a"]`,
		`items[*!key=[1]].value`,
		`items[*!key="a"].value`,
		`items[*!=["a"]].value`,
		`items[*!key].value`,
		`items[*!key=["a"]]tail.value`,
		`"q"[*!key=["a"]]tail`,
		"items[*!key=[\"a\\u0000b\"]].value",
	} {
		if _, err := Parse(s); err == nil {
			t.Fatalf("Parse(%q) succeeded, want an error", s)
		}
	}
}

func TestExceptWhere(t *testing.T) {
	p, err := ExceptWhere("metadata.items[*]", "key", []string{"enable-oslogin"}, "value")
	if err != nil {
		t.Fatal(err)
	}
	if want := `metadata.items[*!key=["enable-oslogin"]].value`; p.String() != want {
		t.Fatalf("String() = %q, want %q", p.String(), want)
	}
	q, err := ExceptWhere("items[*]", "odd.field", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := `items[*!"odd.field"=[]]`; q.String() != want {
		t.Fatalf("String() = %q, want %q", q.String(), want)
	}
	if _, err := ExceptWhere("items", "key", nil, "value"); err == nil {
		t.Fatal("each without [*] must be an error")
	}
	if _, err := ExceptWhere("items[*]", "", nil, "value"); err == nil {
		t.Fatal("an empty field must be an error")
	}
}

func TestPlainPathsUnchanged(t *testing.T) {
	for s, want := range map[string]int{
		"a[*]b[*]":   2, // key "a[*]b", then one wildcard
		"a[0].b":     2,
		"x[*][*].y":  4,
		"weird[key]": 1,
	} {
		p, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if len(p.steps) != want {
			t.Fatalf("Parse(%q) has %d steps, want %d", s, len(p.steps), want)
		}
	}
}
