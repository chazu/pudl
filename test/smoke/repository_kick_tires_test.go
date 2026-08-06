//go:build smoke

package smoke

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSmoke_RepositoryKickTires(t *testing.T) {
	requireTools(t, "git", "mu", "python3")

	w := newKickTiresWorkspace(t)
	if stdout, stderr, err := w.pudl("model", "validate", "kick-consumer"); err != nil {
		t.Fatalf("validate bound model template: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	stdout, stderr, err := w.pudl("run-set", "kick-consumer", "kick-producer", "--mu-root", w.muRoot)
	if err != nil {
		t.Fatalf("reverse-order producer/consumer run-set failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	var report struct {
		Status         string   `json:"status"`
		OrderedMembers []string `json:"ordered_members"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode run-set report: %v\n%s", err, stdout)
	}
	if report.Status != "succeeded" {
		t.Fatalf("run-set status = %q, want succeeded", report.Status)
	}
	want := []string{"kick-producer", "kick-consumer"}
	if fmt.Sprint(report.OrderedMembers) != fmt.Sprint(want) {
		t.Fatalf("ordered members = %v, want %v", report.OrderedMembers, want)
	}

	if err := os.Remove(w.sentinel); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	stdout, stderr, err = w.pudl("run", "kick-consumer", "--mu-root", w.muRoot)
	if err != nil {
		t.Fatalf("standalone consumer reuse failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	sentinel, err := os.ReadFile(w.sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sentinel, []byte("kick-producer")) {
		t.Fatalf("standalone consumer started producer:\n%s", sentinel)
	}
	if !bytes.Contains(sentinel, []byte("kick-consumer")) {
		t.Fatalf("standalone consumer did not invoke its plugin:\n%s", sentinel)
	}
}

func TestSmoke_RepositoryKickTiresFailFast(t *testing.T) {
	requireTools(t, "git", "mu", "python3")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing producer", args: []string{"run-set", "kick-consumer"}, want: "outside the explicit run set"},
		{name: "dependency cycle", args: []string{"run-set", "kick-cycle-a", "kick-cycle-b"}, want: "dependency cycle"},
		{name: "invalid pointer", args: []string{"run-set", "kick-invalid-pointer", "kick-producer"}, want: "RFC 6901 JSON Pointer"},
		{name: "unannotated input", args: []string{"run-set", "kick-unannotated-input", "kick-producer"}, want: "must declare @pudl(binding=plain)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := newKickTiresWorkspace(t)
			stdout, stderr, err := w.pudl(test.args...)
			if err == nil {
				t.Fatalf("command unexpectedly succeeded:\n%s", stdout)
			}
			if !strings.Contains(stderr, test.want) {
				t.Fatalf("stderr does not contain %q:\n%s", test.want, stderr)
			}
			if _, statErr := os.Stat(w.sentinel); !os.IsNotExist(statErr) {
				t.Fatalf("preflight failure invoked plugin; stat error = %v", statErr)
			}
		})
	}
}

func TestSmoke_RepositoryKickTiresFailurePropagation(t *testing.T) {
	requireTools(t, "git", "mu", "python3")
	w := newKickTiresWorkspace(t)

	stdout, stderr, err := w.pudl("run-set", "kick-denied-source", "kick-producer", "--mu-root", w.muRoot)
	if err == nil {
		t.Fatalf("unauthorized projection unexpectedly succeeded:\n%s", stdout)
	}
	if !strings.Contains(stdout, "projection-denied") {
		t.Fatalf("projection failure lacks structured code:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	sentinel, readErr := os.ReadFile(w.sentinel)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Contains(sentinel, []byte("kick-producer")) || bytes.Contains(sentinel, []byte("kick-denied-source")) {
		t.Fatalf("projection should run producer but not consumer:\n%s", sentinel)
	}

	if err := os.Remove(w.sentinel); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err = w.pudlWithEnv(map[string]string{"PUDL_KICK_FAIL_ROLE": "producer"},
		"run-set", "kick-consumer", "kick-producer", "--mu-root", w.muRoot)
	if err == nil {
		t.Fatalf("failed producer run-set unexpectedly succeeded:\n%s", stdout)
	}
	var report struct {
		Members []struct {
			Model  string `json:"model"`
			Result string `json:"result"`
		} `json:"members"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode failed-producer report: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	results := map[string]string{}
	for _, member := range report.Members {
		results[member.Model] = member.Result
	}
	if results["kick-producer"] != "failed" || results["kick-consumer"] != "blocked" {
		t.Fatalf("unexpected failure propagation: %v", results)
	}
}

func TestSmoke_RepositoryKickTiresApprovals(t *testing.T) {
	requireTools(t, "git", "mu", "python3")

	t.Run("resume exact plan in a new process", func(t *testing.T) {
		w := newKickTiresWorkspace(t)
		state := filepath.Join(w.muRoot, "state", "mutator")
		stdout, stderr, err := w.pudl("run-set", "kick-mutator", "--converge", "--require-approval", "--mu-root", w.muRoot)
		if err != nil {
			t.Fatalf("create pending approval: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		report := decodeKickRunSetReport(t, stdout)
		if report.Status != "pending-approval" || report.ApprovalStatus != "pending" {
			t.Fatalf("unexpected pending report: %+v", report)
		}
		if _, statErr := os.Stat(state); !os.IsNotExist(statErr) {
			t.Fatalf("pending plan mutated state; stat error = %v", statErr)
		}

		stdout, stderr, err = w.pudl("run-set", "resume", report.RunSetID)
		if err != nil {
			t.Fatalf("resume exact plan: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		resumed := decodeKickRunSetReport(t, stdout)
		if resumed.Status != "succeeded" || resumed.ApprovalStatus != "approved" {
			t.Fatalf("unexpected resumed report: %+v", resumed)
		}
		if _, statErr := os.Stat(state); statErr != nil {
			t.Fatalf("approved plan did not mutate state: %v", statErr)
		}
	})

	t.Run("changed plan is stale", func(t *testing.T) {
		w := newKickTiresWorkspace(t)
		state := filepath.Join(w.muRoot, "state", "mutator-stale")
		stdout, stderr, err := w.pudlWithEnv(map[string]string{"PUDL_KICK_PLAN_VARIANT": "A"},
			"run-set", "kick-mutator-stale", "--converge", "--require-approval", "--mu-root", w.muRoot)
		if err != nil {
			t.Fatalf("create stale-plan fixture: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		report := decodeKickRunSetReport(t, stdout)
		stdout, stderr, err = w.pudlWithEnv(map[string]string{"PUDL_KICK_PLAN_VARIANT": "B"},
			"run-set", "resume", report.RunSetID)
		if err == nil || !strings.Contains(stderr, "approval is stale") {
			t.Fatalf("changed plan was not rejected as stale: err=%v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		if _, statErr := os.Stat(state); !os.IsNotExist(statErr) {
			t.Fatalf("stale plan mutated state; stat error = %v", statErr)
		}
	})

	t.Run("explicit rejection does not mutate", func(t *testing.T) {
		w := newKickTiresWorkspace(t)
		state := filepath.Join(w.muRoot, "state", "mutator-stale")
		stdout, stderr, err := w.pudl("run-set", "kick-mutator-stale", "--converge", "--require-approval", "--mu-root", w.muRoot)
		if err != nil {
			t.Fatalf("create rejection fixture: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		report := decodeKickRunSetReport(t, stdout)
		stdout, stderr, err = w.pudl("run-set", "reject", report.RunSetID)
		if err != nil {
			t.Fatalf("reject plan: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		rejected := decodeKickRunSetReport(t, stdout)
		if rejected.ApprovalStatus != "rejected" {
			t.Fatalf("unexpected rejected report: %+v", rejected)
		}
		if _, statErr := os.Stat(state); !os.IsNotExist(statErr) {
			t.Fatalf("rejected plan mutated state; stat error = %v", statErr)
		}
	})
}

func TestSmoke_RepositoryKickTiresSealedRouting(t *testing.T) {
	requireTools(t, "git", "mu", "python3")

	t.Run("single model provider write survives approval resume", func(t *testing.T) {
		w := newKickTiresWorkspace(t)
		state := filepath.Join(w.muRoot, "state", "sealed-producer")
		stdout, stderr, err := w.pudl("run", "kick-sealed-producer", "--converge", "--require-approval", "--mu-root", w.muRoot)
		if err != nil {
			t.Fatalf("create sealed approval: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		pending := decodeKickRunReport(t, stdout)
		if !pending.PendingApproval || pending.ApprovalStatus != "pending" {
			t.Fatalf("unexpected pending sealed report: %+v", pending)
		}
		if _, statErr := os.Stat(state); !os.IsNotExist(statErr) {
			t.Fatalf("pending sealed plan mutated state; stat error = %v", statErr)
		}

		stdout, stderr, err = w.pudl("run", "resume", pending.RunID)
		if err != nil {
			t.Fatalf("resume sealed approval: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		resumed := decodeKickRunReport(t, stdout)
		if resumed.CompletionStatus != "succeeded" || resumed.ApprovalStatus != "approved" {
			t.Fatalf("unexpected resumed sealed report: %+v", resumed)
		}
		secrets, readErr := os.ReadFile(w.secretStore)
		if readErr != nil {
			t.Fatalf("read fake provider store: %v", readErr)
		}
		if !bytes.Contains(secrets, []byte(`"token"`)) {
			t.Fatalf("provider store lacks expected key: %s", secrets)
		}
		catalog, readErr := os.ReadFile(filepath.Join(w.root, ".pudl", "data", "sqlite", "catalog.db"))
		if readErr != nil {
			t.Fatalf("read repository catalog: %v", readErr)
		}
		if bytes.Contains(catalog, []byte("kick-token-value")) {
			t.Fatal("resolved secret value leaked into repository catalog")
		}
	})

	t.Run("sealed producer consumer run set survives approval resume", func(t *testing.T) {
		w := newKickTiresWorkspace(t)
		producerState := filepath.Join(w.muRoot, "state", "sealed-producer")
		consumerState := filepath.Join(w.muRoot, "state", "sealed-consumer")
		stdout, stderr, err := w.pudl(
			"run-set", "kick-sealed-consumer", "kick-sealed-producer",
			"--converge", "--mu-root", w.muRoot,
		)
		if err != nil {
			t.Fatalf("create sealed run-set approval: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		pending := decodeKickRunSetReport(t, stdout)
		if pending.Status != "pending-approval" || pending.ApprovalStatus != "pending" {
			t.Fatalf("unexpected pending sealed run-set: %+v", pending)
		}
		for _, path := range []string{producerState, consumerState, w.secretStore} {
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("pending sealed run-set mutated %s; stat error = %v", path, statErr)
			}
		}

		stdout, stderr, err = w.pudl("run-set", "resume", pending.RunSetID)
		if err != nil {
			t.Fatalf("resume sealed run-set: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		resumed := decodeKickRunSetReport(t, stdout)
		if resumed.Status != "succeeded" || resumed.ApprovalStatus != "approved" {
			t.Fatalf("unexpected resumed sealed run-set: %+v", resumed)
		}
		for _, path := range []string{producerState, consumerState} {
			if _, statErr := os.Stat(path); statErr != nil {
				t.Fatalf("approved sealed run-set did not mutate %s: %v", path, statErr)
			}
		}
		secrets, readErr := os.ReadFile(w.secretStore)
		if readErr != nil {
			t.Fatalf("read fake provider store: %v", readErr)
		}
		if !bytes.Contains(secrets, []byte(`"token"`)) {
			t.Fatalf("provider store lacks expected key: %s", secrets)
		}
		if strings.Contains(stdout+stderr, "kick-token-value") {
			t.Fatalf("secret value leaked to process output:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
		}
		catalog, readErr := os.ReadFile(filepath.Join(w.root, ".pudl", "data", "sqlite", "catalog.db"))
		if readErr != nil {
			t.Fatalf("read repository catalog: %v", readErr)
		}
		for _, forbidden := range [][]byte{[]byte("kick-token-value"), []byte("kicksecret:token")} {
			if bytes.Contains(catalog, forbidden) {
				t.Fatalf("sealed material %q leaked into repository catalog", forbidden)
			}
		}
	})

	for _, test := range []struct {
		name    string
		models  []string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "unused input declaration",
			models:  []string{"kick-sealed-consumer", "kick-sealed-producer"},
			env:     map[string]string{"PUDL_KICK_DROP_CLAIMS": "input"},
			wantErr: `declared input "TOKEN" is not claimed`,
		},
		{
			name:    "unused output declaration",
			models:  []string{"kick-sealed-producer"},
			env:     map[string]string{"PUDL_KICK_DROP_CLAIMS": "output"},
			wantErr: `declared output "TOKEN" is not claimed`,
		},
		{
			name:    "undeclared input claim",
			models:  []string{"kick-sealed-producer"},
			env:     map[string]string{"PUDL_KICK_EXTRA_CLAIM": "input"},
			wantErr: `claims undeclared input "UNDECLARED"`,
		},
		{
			name:    "undeclared output claim",
			models:  []string{"kick-sealed-producer"},
			env:     map[string]string{"PUDL_KICK_EXTRA_CLAIM": "output"},
			wantErr: `claims undeclared output "UNDECLARED"`,
		},
		{
			name:    "ambiguous output claim",
			models:  []string{"kick-sealed-producer"},
			env:     map[string]string{"PUDL_KICK_DUPLICATE_OUTPUT_CLAIM": "1"},
			wantErr: `output "TOKEN" is claimed by 2 actions`,
		},
	} {
		t.Run(test.name+" fails before mutation or provider traffic", func(t *testing.T) {
			w := newKickTiresWorkspace(t)
			args := append([]string{"run-set"}, test.models...)
			args = append(args, "--converge", "--mu-root", w.muRoot)
			stdout, stderr, err := w.pudlWithEnv(test.env, args...)
			if err == nil || !strings.Contains(stdout+stderr, test.wantErr) {
				t.Fatalf("strict routing did not reject %s: err=%v\nstdout:\n%s\nstderr:\n%s", test.name, err, stdout, stderr)
			}
			for _, path := range []string{
				filepath.Join(w.muRoot, "state", "sealed-producer"),
				filepath.Join(w.muRoot, "state", "sealed-consumer"),
				w.secretStore,
			} {
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("strict-routing rejection mutated %s; stat error = %v", path, statErr)
				}
			}
			traffic, readErr := os.ReadFile(w.sentinel)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			if bytes.Contains(traffic, []byte("resolve_secret")) || bytes.Contains(traffic, []byte("store_secret")) {
				t.Fatalf("strict-routing rejection reached provider traffic:\n%s", traffic)
			}
		})
	}

	t.Run("workspace policy denies output before plugin traffic", func(t *testing.T) {
		w := newKickTiresWorkspace(t)
		stdout, stderr, err := w.pudl("run-set", "kick-denied-output", "--converge", "--mu-root", w.muRoot)
		if err == nil || !strings.Contains(stdout, "provider reference is not allowed") {
			t.Fatalf("denied output did not fail policy: err=%v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		if _, statErr := os.Stat(w.sentinel); !os.IsNotExist(statErr) {
			t.Fatalf("denied output invoked plugin; stat error = %v", statErr)
		}
	})
}

func TestSmoke_RepositoryKickTiresConcurrentRunSets(t *testing.T) {
	requireTools(t, "git", "mu", "python3")
	w := newKickTiresWorkspace(t)
	type result struct {
		stdout string
		stderr string
		err    error
	}
	results := make(chan result, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			stdout, stderr, err := w.pudl("run-set", "kick-consumer", "kick-producer", "--mu-root", w.muRoot)
			results <- result{stdout: stdout, stderr: stderr, err: err}
		}()
	}
	group.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Errorf("concurrent run-set failed: %v\nstdout:\n%s\nstderr:\n%s", result.err, result.stdout, result.stderr)
			continue
		}
		report := decodeKickRunSetReport(t, result.stdout)
		if report.Status != "succeeded" {
			t.Errorf("concurrent run-set status = %q, want succeeded", report.Status)
		}
	}
	if t.Failed() {
		return
	}
	stdout, stderr, err := w.pudl("doctor")
	if err != nil {
		t.Fatalf("doctor after concurrent run-sets: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
}
