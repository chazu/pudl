package importer

import (
	"testing"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/systemmodel"
)

func TestBuiltInSchemasExposeOnlyExplicitPlainBindingFields(t *testing.T) {
	inferrer, err := inference.NewSchemaInferrer(gitSchemaDir(t))
	if err != nil {
		t.Fatalf("NewSchemaInferrer: %v", err)
	}

	for _, tc := range []struct {
		schema string
		path   string
		want   systemmodel.BindingClass
	}{
		{schema: "pudl/aws.#Subnet", path: "SubnetId", want: systemmodel.BindingPlain},
		{schema: "pudl/aws.#Subnet", path: "VpcId", want: systemmodel.BindingPlain},
		{schema: "pudl/git.#GitRepository", path: "name", want: systemmodel.BindingPlain},
		{schema: "pudl/k8s.#Resource", path: "metadata.name", want: systemmodel.BindingPlain},
		// Operational state remains non-bindable merely because neighboring
		// resource handles opt into projection.
		{schema: "pudl/aws.#Instance", path: "state", want: ""},
	} {
		t.Run(tc.schema+"/"+tc.path, func(t *testing.T) {
			schema, ok := inferrer.GetSchemaValue(tc.schema)
			if !ok {
				t.Fatalf("schema %q is not loaded", tc.schema)
			}
			field := schema.LookupPath(cue.ParsePath(tc.path))
			if !field.Exists() {
				t.Fatalf("field %q does not exist", tc.path)
			}
			got, err := systemmodel.ClassifyBinding(field)
			if err != nil {
				t.Fatalf("ClassifyBinding: %v", err)
			}
			if got != tc.want {
				t.Errorf("binding class = %q, want %q", got, tc.want)
			}
		})
	}
}
