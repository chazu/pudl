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

// TestMuTargetInference verifies a real `mu target list --json` record
// classifies as pudl/mu.#Target. The decisive disambiguation from
// pudl/brick.#Target is the identity field: mu emits `target`, brick keys on
// `name`, so a target record scores on mu.#Target and not brick.
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

// TestMuTargetVsBrickTarget guards the inference boundary: a brick-style record
// (keyed on `name`, with `kind`) must NOT be captured by pudl/mu.#Target, and a
// mu-style record (keyed on `target`) must NOT be captured by brick.
func TestMuTargetVsBrickTarget(t *testing.T) {
	inferrer, err := inference.NewSchemaInferrer(gitSchemaDir(t))
	if err != nil {
		t.Fatalf("NewSchemaInferrer: %v", err)
	}

	brickRecord := map[string]interface{}{
		"name":      "//k8s/api",
		"kind":      "component",
		"toolchain": "k8s",
		"config":    map[string]interface{}{},
	}
	res, err := inferrer.Infer(brickRecord, inference.InferenceHints{})
	if err != nil {
		t.Fatalf("Infer brick: %v", err)
	}
	if res.Schema == "pudl/mu.#Target" {
		t.Errorf("brick-style record (keyed on name) must not classify as pudl/mu.#Target")
	}

	muRecord := map[string]interface{}{
		"target":    "//cmd/mu",
		"toolchain": "go",
	}
	res, err = inferrer.Infer(muRecord, inference.InferenceHints{})
	if err != nil {
		t.Fatalf("Infer mu: %v", err)
	}
	if res.Schema == "pudl/brick.#Target" {
		t.Errorf("mu-style record (keyed on target) must not classify as pudl/brick.#Target")
	}
}
