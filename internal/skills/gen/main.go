// Command gen syncs the canonical skill sources in skills/<name>/SKILL.md into
// the embedded copies in internal/skills/files/<name>.md.
//
// The embedded files are what `pudl init` writes; the skills/ directory is the
// human-facing source of truth (also symlinked from .claude/skills/). These two
// must never drift, so this generator is the single writer of the embedded copy.
//
// Usage:
//
//	go run ./internal/skills/gen          # sync: write embedded copies
//	go run ./internal/skills/gen -check   # verify: exit non-zero if out of sync
//
// It is wired up via `//go:generate` in internal/skills/skills.go and guarded in
// CI (`make check-skills`) plus a unit test (TestEmbeddedSkillsInSync).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	check := flag.Bool("check", false, "verify embedded copies are in sync instead of writing them")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fatalf("locating repo root: %v", err)
	}

	srcGlob := filepath.Join(root, "skills", "*", "SKILL.md")
	matches, err := filepath.Glob(srcGlob)
	if err != nil {
		fatalf("globbing %s: %v", srcGlob, err)
	}
	if len(matches) == 0 {
		fatalf("no skill sources found at %s", srcGlob)
	}

	destDir := filepath.Join(root, "internal", "skills", "files")
	var drift []string
	for _, src := range matches {
		// skills/<name>/SKILL.md -> files/<name>.md
		name := filepath.Base(filepath.Dir(src))
		dest := filepath.Join(destDir, name+".md")

		want, err := os.ReadFile(src)
		if err != nil {
			fatalf("reading %s: %v", src, err)
		}

		if *check {
			got, err := os.ReadFile(dest)
			if err != nil || !bytes.Equal(got, want) {
				drift = append(drift, name)
			}
			continue
		}

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			fatalf("creating %s: %v", destDir, err)
		}
		if err := os.WriteFile(dest, want, 0o644); err != nil {
			fatalf("writing %s: %v", dest, err)
		}
		fmt.Printf("synced %s -> %s\n", relTo(root, src), relTo(root, dest))
	}

	if *check && len(drift) > 0 {
		fatalf("embedded skills out of sync: %v\n  run `go generate ./internal/skills` (or `make generate`) and commit the result", drift)
	}
}

// repoRoot walks up from the working directory until it finds go.mod, so the
// generator works whether invoked from the repo root (`go run ./internal/...`)
// or from internal/skills (`go generate`).
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

func relTo(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen: "+format+"\n", args...)
	os.Exit(1)
}
