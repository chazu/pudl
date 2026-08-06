//go:build smoke

package smoke

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type kickTiresWorkspace struct {
	root        string
	muRoot      string
	home        string
	sentinel    string
	secretStore string
}

type kickRunSetReport struct {
	RunSetID       string `json:"run_set_id"`
	Status         string `json:"status"`
	ApprovalStatus string `json:"approval_status"`
}

type kickRunReport struct {
	RunID            string `json:"run_id"`
	Mode             string `json:"mode"`
	CompletionStatus string `json:"completion_status"`
	PendingApproval  bool   `json:"pending_approval"`
	ApprovalStatus   string `json:"approval_status"`
}

func decodeKickRunSetReport(t *testing.T, stdout string) kickRunSetReport {
	t.Helper()
	var report kickRunSetReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode run-set report: %v\n%s", err, stdout)
	}
	if report.RunSetID == "" {
		t.Fatalf("run-set report has no id:\n%s", stdout)
	}
	return report
}

func decodeKickRunReport(t *testing.T, stdout string) kickRunReport {
	t.Helper()
	var report kickRunReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode run report: %v\n%s", err, stdout)
	}
	if report.RunID == "" {
		t.Fatalf("run report has no id:\n%s", stdout)
	}
	return report
}

func newKickTiresWorkspace(t *testing.T) *kickTiresWorkspace {
	t.Helper()
	repo, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(repo, ".pudl", "data", "kick-tires", "test-runs")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "workspace-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	w := &kickTiresWorkspace{
		root:        root,
		muRoot:      filepath.Join(root, ".pudl", "data", "kick-tires", "mu"),
		home:        filepath.Join(root, ".pudl", "data", "kick-tires", "home"),
		sentinel:    filepath.Join(root, ".pudl", "data", "kick-tires", "sentinel.log"),
		secretStore: filepath.Join(root, ".pudl", "data", "kick-tires", "secrets.json"),
	}
	if _, stderr, err := w.pudl("repo", "init"); err != nil {
		t.Fatalf("pudl repo init: %v\n%s", err, stderr)
	}
	installKickTiresFixtures(t, repo, root)
	if _, stderr, err := w.pudl("repo", "init"); err != nil {
		t.Fatalf("idempotent pudl repo init: %v\n%s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".pudl", "schema", "models", "kick_tires.cue")); err != nil {
		t.Fatalf("repo init did not preserve authored fixture: %v", err)
	}
	if err := os.MkdirAll(w.muRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.muRoot, "mu.cue"), []byte("package mu\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return w
}

func installKickTiresFixtures(t *testing.T, repo, workspace string) {
	t.Helper()
	for _, rel := range []string{
		filepath.Join(".pudl", "schema", "kicktires", "resources.cue"),
		filepath.Join(".pudl", "schema", "models", "kick_tires.cue"),
		filepath.Join(".pudl", "populators", "kick-observe", "plugin.py"),
	} {
		contents, err := os.ReadFile(filepath.Join(repo, rel))
		if err != nil {
			t.Fatalf("read fixture %s: %v", rel, err)
		}
		destination := filepath.Join(workspace, rel)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if filepath.Ext(destination) == ".py" {
			mode = 0o755
		}
		if err := os.WriteFile(destination, contents, mode); err != nil {
			t.Fatalf("write fixture %s: %v", rel, err)
		}
	}
	workspaceCUE := `name: "kick-tires-test"
secrets: writable_refs: ["kicksecret:token"]
`
	if err := os.WriteFile(filepath.Join(workspace, ".pudl", "workspace.cue"), []byte(workspaceCUE), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (w *kickTiresWorkspace) pudl(args ...string) (string, string, error) {
	return w.pudlWithEnv(nil, args...)
}

func (w *kickTiresWorkspace) pudlWithEnv(overrides map[string]string, args ...string) (string, string, error) {
	cmd := exec.Command(pudlBin, append([]string{"--json"}, args...)...)
	cmd.Dir = w.root
	environment := map[string]string{
		"HOME":                   w.home,
		"PUDL_KICK_SECRET_STORE": w.secretStore,
		"PUDL_KICK_SENTINEL":     w.sentinel,
	}
	for key, value := range overrides {
		environment[key] = value
	}
	cmd.Env = envWith(environment)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
