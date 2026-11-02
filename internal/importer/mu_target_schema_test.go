package importer

import (
	"testing"

	"github.com/chazu/pudl/internal/inference"
)

// TestMuTargetRegistration verifies pudl/mu.#Target is registered as a built-in
// schema with identity on `target`.
func TestMuTargetRegistration(t *testing.T) {
	inferrer, err := inference.NewSchemaInferrer(gitSchemaDir(t))
	if err != nil {
		t.Fatalf("NewSchemaInferrer: %v", err)
	}

	if !contains(inferrer.GetAvailableSchemas(), "pudl/mu.#Target") {
		t.Fatalf("expected pudl/mu.#Target to be registered; got %v", inferrer.GetAvailableSchemas())
	}

	meta, ok := inferrer.GetSchemaMetadata("pudl/mu.#Target")
	if !ok {
		t.Fatal("no metadata for pudl/mu.#Target")
	}
	if len(meta.IdentityFields) != 1 || meta.IdentityFields[0] != "target" {
		t.Errorf("identity_fields must be [target], got %v", meta.IdentityFields)
	}
	if meta.ResourceType != "mu.target" {
		t.Errorf("resource_type = %q, want mu.target", meta.ResourceType)
	}
}

func TestMuPlanV2BootstrapSchemaCoversExactExecutionIdentity(t *testing.T) {
	plan := map[string]interface{}{
		"version": 2, "plan_sha256": "0123456789abcdef", "targets": []interface{}{"//app"},
		"plugins": []interface{}{map[string]interface{}{
			"name": "apply", "digest": "sha256:abc", "version": "1.2.3",
			"protocol_version": 1, "capabilities": []interface{}{"plan", "store_secret"},
		}},
		"actions": []interface{}{map[string]interface{}{
			"id": "//app:apply", "action_key": "sha256:def", "command": nil,
			"body": []interface{}{"apply"}, "inputs": map[string]interface{}{"desired": "sha256:123"},
			"outputs": []interface{}{"receipt.json"}, "depends_on": []interface{}{},
			"env":                 map[string]interface{}{"MODE": "exact"},
			"sealed_inputs":       map[string]interface{}{"TOKEN": "fake:apps/token"},
			"sealed_input_modes":  map[string]interface{}{"TOKEN": "file"},
			"sealed_outputs":      map[string]interface{}{"RESULT": "fake:apps/result"},
			"sealed_output_modes": map[string]interface{}{"RESULT": "create_if_absent"},
			"network":             true, "work_dir": "deploy", "impure": true,
			"timeout_s": 30, "retries": 2, "retry_backoff_ms": 250,
			"toolchain": map[string]interface{}{"bin/apply": "sha256:456"},
			"sources":   []interface{}{"desired.json"},
		}},
		"summary": map[string]interface{}{"total": 1},
	}
	if err := ValidateAgainstBootstrapDef("pudl/mu/mu.cue", "#PlanOutput", plan); err != nil {
		t.Fatalf("current mu plan v2 rejected by bootstrap schema: %v", err)
	}
}

func TestMuTargetBootstrapSchemaCoversStrictSealedRouting(t *testing.T) {
	target := map[string]interface{}{
		"target": "//app", "toolchain": "apply",
		"sealed_outputs":      map[string]interface{}{"RESULT": "fake:apps/result"},
		"sealed_output_modes": map[string]interface{}{"RESULT": "create_if_absent"},
		"sealed_routing":      "strict",
	}
	if err := ValidateAgainstBootstrapDef("pudl/mu/mu.cue", "#Target", target); err != nil {
		t.Fatalf("current strict target rejected by bootstrap schema: %v", err)
	}
}

// TestMuTargetInference verifies a real `mu target list --json` record
// still classifies as the legacy pudl/mu.#Target.
func TestMuTargetInference(t *testing.T) {
	inferrer, err := inference.NewSchemaInferrer(gitSchemaDir(t))
	if err != nil {
		t.Fatalf("NewSchemaInferrer: %v", err)
	}

	cases := []struct {
		label string
		data  map[string]interface{}
		want  string
	}{
		{
			label: "go-target",
			data: map[string]interface{}{
				"target":    "//cmd/mu",
				"toolchain": "go",
				"sources":   []interface{}{"cmd/mu/main.go"},
				"config":    map[string]interface{}{"output": "mu"},
			},
			want: "pudl/mu.#Target",
		},
		{
			label: "pith-planned-target-no-toolchain",
			data: map[string]interface{}{
				"target": "//inventory/gitlab",
				"sealed_inputs": map[string]interface{}{
					"GITLAB_TOKEN": "pass:gitlab/token",
				},
				"plan": []interface{}{"target/config"},
			},
			want: "pudl/mu.#Target",
		},
		{
			// mu target list --json emits JSON null (not an absent key) for a
			// target with no sources. The schema must tolerate null on optional
			// list fields or CUE unification drops #Target as a candidate.
			label: "sources-null-from-cli",
			data: map[string]interface{}{
				"target":             "//inventory/gitlab-repos",
				"sources":            nil,
				"sealed_inputs":      map[string]interface{}{"GITLAB_TOKEN": "env:GITLAB_TOKEN"},
				"sealed_input_modes": map[string]interface{}{"GITLAB_TOKEN": "env"},
				"plan":               []interface{}{map[string]interface{}{"body": []interface{}{}}},
			},
			want: "pudl/mu.#Target",
		},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			res, err := inferrer.Infer(tc.data, inference.InferenceHints{})
			if err != nil {
				t.Fatalf("Infer: %v", err)
			}
			if res.Schema != tc.want {
				t.Errorf("got schema %q (confidence %.2f, reason %q), want %q",
					res.Schema, res.Confidence, res.Reason, tc.want)
			}
		})
	}
}
