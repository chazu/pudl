package fieldpath

import (
	"reflect"
	"testing"
)

func record() map[string]any {
	return map[string]any{
		"name": "fw-1",
		"metadata": map[string]any{
			"name":   "svc",
			"labels": map[string]any{"cloud.googleapis.com/location": "us-east1"},
		},
		"sourceRanges": []any{"0.0.0.0/0", "10.0.0.0/8"},
		"allowed": []any{
			map[string]any{"IPProtocol": "tcp"},
			map[string]any{"IPProtocol": "udp"},
		},
	}
}

func TestParseCompatibleWithDotSplit(t *testing.T) {
	for _, s := range []string{"name", "metadata.name", "a.b.c", "weird-key", "with space.x"} {
		p, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if p.HasWildcard() {
			t.Fatalf("Parse(%q) reported wildcard", s)
		}
	}
}

func TestLookupQuotedSegment(t *testing.T) {
	p := MustParse(`metadata.labels."cloud.googleapis.com/location"`)
	got, ok := p.Lookup(record())
	if !ok || got != "us-east1" {
		t.Fatalf("Lookup = %v, %v", got, ok)
	}
}

func TestLookupAbsentAndWildcard(t *testing.T) {
	if _, ok := MustParse("metadata.missing").Lookup(record()); ok {
		t.Fatal("absent path found")
	}
	if _, ok := MustParse("sourceRanges[*]").Lookup(record()); ok {
		t.Fatal("Lookup must refuse wildcards")
	}
}

func TestLookupAll(t *testing.T) {
	got := MustParse("sourceRanges[*]").LookupAll(record())
	if !reflect.DeepEqual(got, []any{"0.0.0.0/0", "10.0.0.0/8"}) {
		t.Fatalf("sourceRanges[*] = %v", got)
	}
	got = MustParse("allowed[*].IPProtocol").LookupAll(record())
	if !reflect.DeepEqual(got, []any{"tcp", "udp"}) {
		t.Fatalf("allowed[*].IPProtocol = %v", got)
	}
	if got := MustParse("nope[*]").LookupAll(record()); got != nil {
		t.Fatalf("absent wildcard = %v", got)
	}
}

func TestSet(t *testing.T) {
	m := record()
	if err := MustParse("project").Set(m, "p1"); err != nil || m["project"] != "p1" {
		t.Fatalf("Set project: %v %v", err, m["project"])
	}
	if err := MustParse(`a."b.c"`).Set(m, 1); err != nil {
		t.Fatal(err)
	}
	if v, _ := MustParse(`a."b.c"`).Lookup(m); v != 1 {
		t.Fatalf("nested set = %v", v)
	}
	if err := MustParse("name.x").Set(m, 1); err == nil {
		t.Fatal("setting through a scalar must fail")
	}
	if err := MustParse("a[*]").Set(m, 1); err == nil {
		t.Fatal("setting through a wildcard must fail")
	}
}

func TestReplace(t *testing.T) {
	m := record()
	n := MustParse("allowed[*].IPProtocol").Replace(m, func(any) any { return "x" })
	if n != 2 {
		t.Fatalf("replaced %d", n)
	}
	if got := MustParse("allowed[*].IPProtocol").LookupAll(m); !reflect.DeepEqual(got, []any{"x", "x"}) {
		t.Fatalf("after replace = %v", got)
	}
	if n := MustParse("metadata.none").Replace(m, func(any) any { return "x" }); n != 0 {
		t.Fatalf("absent replace = %d", n)
	}
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{"", "a..b", "a.", `"unterminated`, `""`, `"a"b`} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) succeeded", s)
		}
	}
}
