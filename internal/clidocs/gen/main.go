// Command gen writes docs/cli-reference.md from the pudl command tree and the
// hand-written notes in docs/cli/notes/.
//
// Usage:
//
//	go run ./internal/clidocs/gen          # write docs/cli-reference.md
//	go run ./internal/clidocs/gen -check   # exit non-zero if it is stale
//
// It is wired up via `//go:generate` in internal/clidocs/doc.go, `make docs`
// and `make check-docs`, and guarded in CI plus a unit test
// (TestCLIReferenceInSync).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/cmd"
	"github.com/chazu/pudl/internal/clidocs"
)

func main() {
	check := flag.Bool("check", false, "verify docs/cli-reference.md is current instead of writing it")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fatalf("locating repo root: %v", err)
	}
	want, err := clidocs.Render(cmd.RootCommand(), os.DirFS(filepath.Join(root, clidocs.NotesDir)))
	if err != nil {
		fatalf("rendering: %v", err)
	}
	dest := filepath.Join(root, clidocs.ReferencePath)

	if *check {
		got, err := os.ReadFile(dest)
		if err != nil || !bytes.Equal(got, want) {
			fatalf("%s is stale\n  run `make docs` (or `go generate ./internal/clidocs`) and commit the result", clidocs.ReferencePath)
		}
		return
	}
	if err := os.WriteFile(dest, want, 0o644); err != nil {
		fatalf("writing %s: %v", dest, err)
	}
	fmt.Printf("wrote %s\n", clidocs.ReferencePath)
}

// repoRoot walks up from the working directory until it finds go.mod, so the
// generator works from the repo root or from internal/clidocs (go generate).
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "clidocs: "+format+"\n", args...)
	os.Exit(1)
}
