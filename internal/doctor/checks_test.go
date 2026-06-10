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
