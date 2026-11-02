//go:build smoke

// Package smoke holds end-to-end smoke tests that drive a freshly built pudl
// binary in a sandboxed workspace. They are gated behind the `smoke` build tag
// so the normal `go test ./...` never runs them.
//
// Run all:            go test -tags=smoke ./test/smoke/ -v -timeout 20m
// Run the walkthrough: make test-git-walkthrough
package smoke

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// pudlBin is the freshly-built pudl binary under test, set by TestMain.
var pudlBin string

func TestMain(m *testing.M) {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "smoke: locate repo root:", err)
		os.Exit(1)
	}
	binDir := filepath.Join(root, ".pudl", "data", "smoke", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "smoke: create repository-local bin dir:", err)
		os.Exit(1)
	}
	bin := filepath.Join(binDir, fmt.Sprintf("pudl-%d", os.Getpid()))
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "smoke: build pudl failed: %v\n%s", err, out)
		os.Exit(1)
	}
	pudlBin = bin
	code := m.Run()
	_ = os.Remove(bin)
	os.Exit(code)
}

// repoRoot returns the module root (two dirs up from this test file).
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
}

// envWith returns the process environment with the given keys overridden
// (existing copies removed first, so the child sees exactly these values).
func envWith(overrides map[string]string) []string {
	skip := map[string]bool{}
	for k := range overrides {
		skip[k] = true
	}
	var env []string
	for _, e := range os.Environ() {
		if i := strings.IndexByte(e, '='); i >= 0 && skip[e[:i]] {
			continue
		}
		env = append(env, e)
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}
