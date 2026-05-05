package muschemas_test

import (
	"errors"
	"testing"

	"pudl/internal/muschemas"
)

func newCache(t *testing.T) *muschemas.Cache {
	t.Helper()
	c, err := muschemas.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestInsertAndHas(t *testing.T) {
	c := newCache(t)
	files := []muschemas.File{
		{RelPath: "ec2.cue", Content: []byte("package aws\n#EC2Instance: {}\n")},
	}
	if err := c.Insert("mu/aws", "v1", files); err != nil {
		t.Fatal(err)
	}
	if !c.Has("mu/aws", "v1") {
		t.Fatal("expected Has true")
	}
	if c.Has("mu/aws", "v2") {
		t.Fatal("expected Has false for unknown version")
	}
}

func TestInsertIdempotent(t *testing.T) {
	c := newCache(t)
	files := []muschemas.File{{RelPath: "x.cue", Content: []byte("a")}}
	if err := c.Insert("mu/aws", "v1", files); err != nil {
		t.Fatal(err)
	}
	if err := c.Insert("mu/aws", "v1", files); err != nil {
		t.Fatal(err)
	}
}

func TestInsertVersionMismatch(t *testing.T) {
	c := newCache(t)
	if err := c.Insert("mu/aws", "v1", []muschemas.File{{RelPath: "x.cue", Content: []byte("a")}}); err != nil {
		t.Fatal(err)
	}
	err := c.Insert("mu/aws", "v1", []muschemas.File{{RelPath: "x.cue", Content: []byte("b")}})
	if !errors.Is(err, muschemas.ErrVersionMismatch) {
		t.Errorf("got %v, want ErrVersionMismatch", err)
	}
}

func TestParseRef(t *testing.T) {
	cases := []struct {
		ref, mod, ver, def string
		ok                 bool
	}{
		{"mu/aws@v1", "mu/aws", "v1", "", true},
		{"mu/aws@v1#EC2Instance", "mu/aws", "v1", "#EC2Instance", true},
		{"pudl/core@v2#Item", "pudl/core", "v2", "#Item", true},
		{"mu/aws", "", "", "", false},
		{"@v1", "", "", "", false},
		{"mu/aws@", "", "", "", false},
	}
	for _, tc := range cases {
		m, v, d, err := muschemas.ParseRef(tc.ref)
		if (err == nil) != tc.ok {
			t.Errorf("%s: ok=%v want %v err=%v", tc.ref, err == nil, tc.ok, err)
			continue
		}
		if !tc.ok {
			continue
		}
		if m != tc.mod || v != tc.ver || d != tc.def {
			t.Errorf("%s: got %q/%q/%q want %q/%q/%q", tc.ref, m, v, d, tc.mod, tc.ver, tc.def)
		}
	}
}
