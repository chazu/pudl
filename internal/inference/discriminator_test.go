package inference

import (
	"os"
	"path/filepath"
	"testing"
)

// discriminatorSchemas pairs a generic schema that matches any record with
// `name` and `kind` (shaped like pudl/infra.#Service) with a specific type
// whose identity is selfLink and whose kind is pinned to one value.
const discriminatorSchemas = `package x

#Service: {
	_pudl: {schema_type: "base", resource_type: "x.service", identity_fields: ["name"], tracked_fields: ["kind"]}
	name: string
	kind: string
	...
}

#Firewall: {
	_pudl: {schema_type: "base", resource_type: "x.firewall", identity_fields: ["selfLink"]}
	kind:     "compute#firewall"
	name:     string
	selfLink: string
	...
}
`

// TestInferPrefersPinnedDiscriminator: identity plus tracked fields give the
// generic schema the higher name-only score, so without the discriminator the
// generic schema is tried, and accepted, first.
func TestInferPrefersPinnedDiscriminator(t *testing.T) {
	dir := t.TempDir()
	setupMinimalCueMod(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "x"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x", "x.cue"), []byte(discriminatorSchemas), 0644); err != nil {
		t.Fatal(err)
	}
	si, err := NewSchemaInferrer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if meta, _ := si.GetSchemaMetadata("x.#Firewall"); meta.Discriminators["kind"] != "compute#firewall" {
		t.Fatalf("discriminator not extracted: %v (load errors %v)", meta.Discriminators, si.LoadErrors())
	}
	if meta, _ := si.GetSchemaMetadata("x.#Service"); len(meta.Discriminators) != 0 {
		t.Fatalf("open fields are not discriminators: %v", meta.Discriminators)
	}

	for name, tc := range map[string]struct {
		data map[string]any
		want string
	}{
		"pinned kind matches": {map[string]any{"kind": "compute#firewall", "name": "fw", "selfLink": "l"}, "x.#Firewall"},
		"other kind":          {map[string]any{"kind": "compute#route", "name": "r", "selfLink": "l"}, "x.#Service"},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := si.Infer(tc.data, InferenceHints{})
			if err != nil {
				t.Fatal(err)
			}
			if result.Schema != tc.want {
				t.Fatalf("inferred %s, want %s (cascade %v)", result.Schema, tc.want, result.CascadePath)
			}
		})
	}
}
