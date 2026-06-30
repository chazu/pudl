//go:build smoke

// Package smoke holds end-to-end smoke tests for the convergence + cross-model
// dependency surface. They are gated behind the `smoke` build tag so the normal
// `go test ./...` never runs them, and each skips cleanly when its required
// tooling (docker / k3d / kubectl / mu / bb / jq) is absent. Every test cleans
// up after itself (clusters, containers, temp dirs) via t.Cleanup.
//
// Run all:           go test -tags=smoke ./test/smoke/ -v -timeout 20m
// Run one:           go test -tags=smoke ./test/smoke/ -v -run TestSmoke_CrossModelDeps
// Or via make:       make smoke
//
// The infra tests need a mu project to borrow toolchains from. By default they
// look for ~/dev/go/mu; override with PUDL_SMOKE_MU_ROOT=/path/to/mu-project.
package smoke

import (
	"fmt"
	"io/fs"
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
	bin := filepath.Join(os.TempDir(), fmt.Sprintf("pudl-smoke-%d", os.Getpid()))
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

// ---- shared helpers ---------------------------------------------------------

// requireTools skips the test unless every named tool is on PATH.
func requireTools(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if _, err := exec.LookPath(n); err != nil {
			t.Skipf("smoke: required tool %q not on PATH; skipping", n)
		}
	}
}

// requireDockerDaemon skips unless the docker daemon is reachable.
func requireDockerDaemon(t *testing.T) {
	t.Helper()
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("smoke: docker daemon not reachable; skipping (%v)\n%s", err, out)
	}
}

// isolatedHome creates a throwaway HOME and registers its cleanup (CUE module
// caches drop read-only files, so chmod first).
func isolatedHome(t *testing.T) string {
	t.Helper()
	h, err := os.MkdirTemp("", "pudl-smoke-home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(h, func(p string, d fs.DirEntry, err error) error {
			if err == nil {
				_ = os.Chmod(p, 0o755)
			}
			return nil
		})
		_ = os.RemoveAll(h)
	})
	return h
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

// pudl runs the binary with HOME isolated, returning combined output + error.
func pudl(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(pudlBin, args...)
	cmd.Env = envWith(map[string]string{"HOME": home})
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// pudlOK runs pudl and fails the test on error.
func pudlOK(t *testing.T, home string, args ...string) string {
	t.Helper()
	out, err := pudl(t, home, args...)
	if err != nil {
		t.Fatalf("pudl %v failed: %v\n%s", args, err, out)
	}
	return out
}

// writeModel writes a model .cue into the isolated schema's models/ dir.
func writeModel(t *testing.T, home, name, body string) {
	t.Helper()
	dir := filepath.Join(home, ".pudl", "schema", "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustContain(t *testing.T, out, want, ctx string) {
	t.Helper()
	if !strings.Contains(out, want) {
		t.Errorf("%s: expected output to contain %q\n--- output ---\n%s", ctx, want, out)
	}
}

func mustNotContain(t *testing.T, out, bad, ctx string) {
	t.Helper()
	if strings.Contains(out, bad) {
		t.Errorf("%s: expected output NOT to contain %q\n--- output ---\n%s", ctx, bad, out)
	}
}

// realHome is the user's actual home (HOME is only overridden for child pudl
// processes, never the test process), used to reach the warm ~/.mu/cache.
func realHome(t *testing.T) string {
	t.Helper()
	h, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home: %v", err)
	}
	return h
}

// muProjectSrc returns a mu project root to borrow toolchains + the k8s plugin
// from (PUDL_SMOKE_MU_ROOT, else ~/dev/go/mu). Skips if none is usable.
func muProjectSrc(t *testing.T) string {
	t.Helper()
	src := os.Getenv("PUDL_SMOKE_MU_ROOT")
	if src == "" {
		src = filepath.Join(realHome(t), "dev", "go", "mu")
	}
	if _, err := os.Stat(filepath.Join(src, "mu.cue")); err != nil {
		t.Skipf("smoke: no mu project at %s (set PUDL_SMOKE_MU_ROOT); skipping", src)
	}
	return src
}

// minimalMuRoot writes a temp mu project that reuses the source project's
// toolchains but a disk-only cache pointed at the warm ~/.mu/cache — bypassing
// any private OCI cache backend that needs credentials. Returns the temp root.
func minimalMuRoot(t *testing.T, src string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(src, "mu.cue"))
	if err != nil {
		t.Skipf("smoke: read %s/mu.cue: %v", src, err)
	}
	toolchains := extractCueList(string(content), "toolchains")
	if toolchains == "" {
		t.Skip("smoke: could not extract toolchains from mu.cue; skipping")
	}
	cache := filepath.Join(realHome(t), ".mu", "cache")
	dir := t.TempDir()
	mucue := fmt.Sprintf("package mu\n\ncache: backends: [{type: \"disk\", path: %q}]\n\ntoolchains: %s\n", cache, toolchains)
	if err := os.WriteFile(filepath.Join(dir, "mu.cue"), []byte(mucue), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// pidish returns the process id, used to make cluster/container names unique.
func pidish() int { return os.Getpid() }

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// extractCueList returns the bracketed list literal that follows `key:` in CUE
// source (matching nested brackets). Returns "" if not found.
func extractCueList(content, key string) string {
	idx := strings.Index(content, key+":")
	if idx < 0 {
		return ""
	}
	rest := content[idx+len(key)+1:]
	start := strings.IndexByte(rest, '[')
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(rest); i++ {
		switch rest[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return rest[start : i+1]
			}
		}
	}
	return ""
}

// ---- no-infra smoke ---------------------------------------------------------

// TestSmoke_CrossModelDeps exercises the cross-model dependency surface with no
// external infra (only the pudl binary): declared discovery pass, derivation,
// queries, idempotency, and retraction.
func TestSmoke_CrossModelDeps(t *testing.T) {
	home := isolatedHome(t)
	pudlOK(t, home, "init")

	// network produces Namespace "foo"; workloads' Deployment references it via
	// metadata.namespace — but declares NO depends_on (must be DERIVED).
	writeModel(t, home, "network.cue", `package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#Network: sm.#SystemModel & {
	name: "network"
	plugins: [{name: "k8s", command: ["bb", "/x"]}]
	populate: {plugin: "k8s", differential: true, input: {}}
	desired: [{apiVersion: "v1", kind: "Namespace", metadata: name: "foo"}]
}
`)
	writeModel(t, home, "workloads.cue", `package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#Workloads: sm.#SystemModel & {
	name: "workloads"
	plugins: [{name: "k8s", command: ["bb", "/x"]}]
	populate: {plugin: "k8s", differential: true, input: {}}
	desired: [{apiVersion: "apps/v1", kind: "Deployment", metadata: {name: "nginx", namespace: "foo"}}]
}
`)

	// Declared discovery pass: no depends_on declared yet -> empty graph.
	out := pudlOK(t, home, "model", "deps")
	mustContain(t, out, "No cross-model dependencies", "declared discovery (none declared)")

	// Derive: the namespace reference yields workloads -> network.
	out = pudlOK(t, home, "model", "deps", "--derive")
	mustContain(t, out, "workloads depends on", "derive")
	mustContain(t, out, "network", "derive target")
	mustContain(t, out, "derived", "derive provenance")

	// Derived edge is queryable.
	out = pudlOK(t, home, "query", "impacted_by", "changed=network")
	mustContain(t, out, "workloads", "impacted_by")

	out = pudlOK(t, home, "query", "--topo", "model_depends_on")
	if i, j := strings.Index(out, "network"), strings.Index(out, "workloads"); i < 0 || j < 0 || i > j {
		t.Errorf("topo order should list network before workloads:\n%s", out)
	}

	// Idempotency: re-derive, still exactly one edge.
	out = pudlOK(t, home, "query", "model_depends_on", "from=workloads", "--json")
	if n := strings.Count(out, "\"from\""); n != 1 {
		t.Errorf("expected 1 derived edge after re-derive, got %d\n%s", n, out)
	}

	// Completion lists the derived rule-head relations.
	out = pudlOK(t, home, "__complete", "query", "")
	for _, rel := range []string{"depends_transitive", "impacted_by", "cyclic"} {
		mustContain(t, out, rel, "completion lists rule heads")
	}
}
