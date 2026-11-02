package inference

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// claimSchemas pairs a generic schema that accepts any record with `name` and
// `kind` (shaped like the removed pudl/infra.#Service) with a type that claims records by
// kind (@pudl(match)) and requires project, and a specialization of that type.
const claimSchemas = `package x

#Service: {
	_pudl: {schema_type: "base", resource_type: "x.service", identity_fields: ["name"]}
	name: string
	kind: string
	...
}

#Firewall: {
	_pudl: {schema_type: "base", resource_type: "x.firewall", identity_fields: ["selfLink"]}
	kind:     "compute#firewall" @pudl(match)
	project:  =~"^[a-z][a-z0-9-]{4,28}[a-z0-9]$"
	name:     string
	selfLink: string
	...
}

#ProdFirewall: {
	_pudl: {schema_type: "custom", resource_type: "x.firewall", base_schema: "x.#Firewall", identity_fields: ["selfLink"]}
	kind:     "compute#firewall"
	project:  =~"^prod-"
	name:     string
	selfLink: string
	...
}

#Topic: {
	_pudl: {schema_type: "base", resource_type: "x.topic", identity_fields: ["name"]}
	project: string
	name:    =~"^projects/[^/]+/topics/" @pudl(match)
	...
}
`

func claimInferrer(t *testing.T) *SchemaInferrer {
	t.Helper()
	dir := t.TempDir()
	setupMinimalCueMod(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "x"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x", "x.cue"), []byte(claimSchemas), 0644); err != nil {
		t.Fatal(err)
	}
	si, err := NewSchemaInferrer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if errs := si.LoadErrors(); len(errs) > 0 {
		t.Fatalf("load errors: %v", errs)
	}
	return si
}

func TestInferAssignsClaimedType(t *testing.T) {
	si := claimInferrer(t)
	for name, tc := range map[string]struct {
		data map[string]any
		want string
	}{
		"claimed by kind":         {map[string]any{"kind": "compute#firewall", "project": "dev-project", "name": "fw", "selfLink": "l"}, "x.#Firewall"},
		"claimed specialization":  {map[string]any{"kind": "compute#firewall", "project": "prod-project", "name": "fw", "selfLink": "l"}, "x.#ProdFirewall"},
		"claimed by name pattern": {map[string]any{"project": "p", "name": "projects/p/topics/t"}, "x.#Topic"},
		"unclaimed kind":          {map[string]any{"kind": "compute#route", "name": "r", "selfLink": "l"}, "x.#Service"},
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

// A record that claims a type but fails it is rejected, never assigned the
// generic schema it would otherwise satisfy.
func TestInferRejectsClaimedRecordThatFailsItsType(t *testing.T) {
	si := claimInferrer(t)
	for name, data := range map[string]map[string]any{
		"missing project":       {"kind": "compute#firewall", "name": "fw", "selfLink": "l"},
		"project number":        {"kind": "compute#firewall", "project": "123456789012", "name": "fw", "selfLink": "l"},
		"project display name":  {"kind": "compute#firewall", "project": "My Project", "name": "fw", "selfLink": "l"},
		"topic without project": {"name": "projects/p/topics/t"},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := si.Infer(data, InferenceHints{})
			var claimErr *ClaimError
			if !errors.As(err, &claimErr) {
				t.Fatalf("want ClaimError, got result %+v err %v", result, err)
			}
			if !si.Claims(data) {
				t.Fatal("Claims should report the claim")
			}
		})
	}
}
