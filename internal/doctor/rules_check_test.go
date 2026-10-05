package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRulesAt(t *testing.T) {
	dir := t.TempDir()
	good := "package rules\n\ng: {\n\thead: {rel: \"g\", args: {id: \"$X\"}}\n\tbody: [{rel: \"s\", args: {id: \"$X\"}}]\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "good.cue"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := CheckRulesAt(dir); res.Status != "ok" {
		t.Fatalf("healthy rules: want ok, got %s: %s", res.Status, res.Details)
	}

	bad := "package rules\n\nb: {\n\thead: {rel: \"b\", args: {id: \"$X\"}}\n\tbody: [{args: {id: \"$X\"}}]\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "bad.cue"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	res := CheckRulesAt(dir)
	if res.Status != "error" || !strings.Contains(res.Details, "bad.cue") {
		t.Fatalf("broken rule: want error naming bad.cue, got %s: %s", res.Status, res.Details)
	}
}
