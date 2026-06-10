package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCUE creates a .cue file (and parents) with trivial content.
func writeCUE(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeSchemaModule scaffolds a loadable CUE module under $HOME/.pudl/schema
// with the given package files. files maps "<pkg>/<file>.cue" -> contents.
func writeSchemaModule(t *testing.T, home string, files map[string]string) {
	t.Helper()
	schema := filepath.Join(home, ".pudl", "schema")
	modFile := filepath.Join(schema, "cue.mod", "module.cue")
	if err := os.MkdirAll(filepath.Dir(modFile), 0755); err != nil {
		t.Fatal(err)
	}
	mod := "module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"
	if err := os.WriteFile(modFile, []byte(mod), 0644); err != nil {
		t.Fatal(err)
	}
	for rel, content := range files {
		p := filepath.Join(schema, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckIdentityFieldConsistency(t *testing.T) {
	const base = `package fam

#Base: {
	_pudl: {
		schema_type: "base"
		resource_type: "fam.base"
		identity_fields: ["id"]
	}
	id: string
	...
}
`

	t.Run("consistent family is ok", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		// Child restates the same identity_fields as its base.
		child := `package fam

#Child: {
	_pudl: {
		schema_type: "policy"
		resource_type: "fam.base"
		base_schema: "fam.#Base"
		identity_fields: ["id"]
	}
	id: string
	...
}
`
		writeSchemaModule(t, home, map[string]string{
			"fam/base.cue":  base,
			"fam/child.cue": child,
		})

		res := CheckIdentityFieldConsistency()
		if res.Status != "ok" {
			t.Fatalf("expected ok, got %q: %s (%s)", res.Status, res.Message, res.Details)
		}
	})

	t.Run("divergent identity_fields warns", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		// Child references the base but declares different identity_fields,
		// bypassing CUE inheritance.
		child := `package fam

#Child: {
	_pudl: {
		schema_type: "base"
		resource_type: "fam.child"
		base_schema: "fam.#Base"
		identity_fields: ["other"]
	}
	other: string
	...
}
`
		writeSchemaModule(t, home, map[string]string{
			"fam/base.cue":  base,
			"fam/child.cue": child,
		})

		res := CheckIdentityFieldConsistency()
		if res.Status != "warning" {
			t.Fatalf("expected warning, got %q: %s (%s)", res.Status, res.Message, res.Details)
		}
		if !strings.Contains(res.Details, "fam.#Child") {
			t.Fatalf("expected details to name fam.#Child, got: %s", res.Details)
		}
	})
}

func TestCheckPudlNamespaceSchemas(t *testing.T) {
	t.Run("no pudl namespace is ok", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		// Create schema dir but no pudl/ subdir.
		if err := os.MkdirAll(filepath.Join(home, ".pudl", "schema"), 0755); err != nil {
			t.Fatal(err)
		}

		res := CheckPudlNamespaceSchemas()
		if res.Status != "ok" {
			t.Fatalf("expected ok, got %q: %s", res.Status, res.Message)
		}
	})

	t.Run("only built-in packages is ok", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		schema := filepath.Join(home, ".pudl", "schema")
		// pudl/core is a built-in bootstrap package.
		writeCUE(t, filepath.Join(schema, "pudl", "core", "core.cue"))

		res := CheckPudlNamespaceSchemas()
		if res.Status != "ok" {
			t.Fatalf("expected ok, got %q: %s (%s)", res.Status, res.Message, res.Details)
		}
	})

	t.Run("user schema under pudl warns", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		schema := filepath.Join(home, ".pudl", "schema")
		// A built-in alongside a user-authored package under pudl/.
		writeCUE(t, filepath.Join(schema, "pudl", "core", "core.cue"))
		writeCUE(t, filepath.Join(schema, "pudl", "myschema", "foo.cue"))

		res := CheckPudlNamespaceSchemas()
		if res.Status != "warning" {
			t.Fatalf("expected warning, got %q: %s", res.Status, res.Message)
		}
		if !strings.Contains(res.Details, "pudl/myschema") {
			t.Fatalf("expected details to name pudl/myschema, got: %s", res.Details)
		}
		// The built-in must not be flagged.
		if strings.Contains(res.Details, "pudl/core") {
			t.Fatalf("built-in pudl/core should not be flagged, got: %s", res.Details)
		}
	})
}
