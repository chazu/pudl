package inference

import (
	"os"
	"path/filepath"
	"testing"
)

// policySchemas has types (one with a custom subtype) and standalone policies
// linked by base_schema. Each policy is deeper than its type and accepts the
// records below, so before policies left classification they won.
const policySchemas = `package x

#Bucket: {
	_pudl: {schema_type: "base", resource_type: "x.bucket", identity_fields: ["name"]}
	name: string
	...
}

#BucketSpecial: {
	_pudl: {schema_type: "custom", resource_type: "x.bucket", base_schema: "x.#Bucket", identity_fields: ["name"]}
	name:    string
	special: string
	...
}

#BucketHardened: {
	_pudl: {schema_type: "policy", base_schema: "x.#Bucket", identity_fields: ["name"]}
	encrypted: true
	...
}

#Thing: {
	_pudl: {schema_type: "base", resource_type: "x.thing"}
	kind: string
	...
}

#ThingPolicy: {
	_pudl: {schema_type: "policy", base_schema: "x.#Thing"}
	kind: "thing"
	...
}
`

// TestInferNeverReturnsPolicy covers both candidate paths: heuristic
// candidates (identity fields present) and the all-schemas fallback taken when
// no heuristic scores (there is no catch-all in this fixture). Type-to-type
// inheritance still selects the most specific type.
func TestInferNeverReturnsPolicy(t *testing.T) {
	dir := t.TempDir()
	setupMinimalCueMod(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "x"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x", "x.cue"), []byte(policySchemas), 0644); err != nil {
		t.Fatal(err)
	}
	si, err := NewSchemaInferrer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if meta, ok := si.GetSchemaMetadata("x.#BucketHardened"); !ok || !meta.IsPolicy() {
		t.Fatalf("fixture policy not loaded as a policy: %+v (load errors %v)", meta, si.LoadErrors())
	}

	for name, tc := range map[string]struct {
		data map[string]any
		want string
	}{
		"heuristic candidates": {map[string]any{"name": "b", "encrypted": true}, "x.#Bucket"},
		"custom subtype":       {map[string]any{"name": "b", "special": "y", "encrypted": true}, "x.#BucketSpecial"},
		"all-schemas fallback": {map[string]any{"kind": "thing"}, "x.#Thing"},
		"mixed fields":         {map[string]any{"kind": "thing", "name": "b"}, "x.#Bucket"},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := si.InferWithTrace(tc.data, InferenceHints{})
			if err != nil {
				t.Fatal(err)
			}
			if result.Schema != tc.want {
				t.Fatalf("inferred %s, want %s", result.Schema, tc.want)
			}
			for _, candidate := range result.CascadePath {
				if meta, _ := si.GetSchemaMetadata(candidate); meta.IsPolicy() {
					t.Fatalf("policy %s was a candidate: %v", candidate, result.CascadePath)
				}
			}
		})
	}
}
