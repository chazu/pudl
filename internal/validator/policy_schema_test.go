package validator

import (
	"errors"
	"strings"
	"testing"
)

// policyChainSchemas is a type, a custom subtype, a policy, a policy extending
// that policy, and a (malformed) type whose base is a policy.
const policyChainSchemas = `package x

#Bucket: {
	_pudl: {schema_type: "base", resource_type: "x.bucket", identity_fields: ["name"]}
	name: string
	...
}

#BucketSpecial: {
	_pudl: {schema_type: "custom", resource_type: "x.bucket", base_schema: "x.#Bucket"}
	name:    string
	special: string
	...
}

#BucketHardened: {
	_pudl: {schema_type: "policy", base_schema: "x.#Bucket"}
	encrypted: true
	...
}

#BucketPCI: {
	_pudl: {schema_type: "policy", base_schema: "x.#BucketHardened"}
	versioned: true
	...
}

#Weird: {
	_pudl: {schema_type: "custom", base_schema: "x.#BucketHardened"}
	...
}
`

func policyChainValidator(t *testing.T) *ChainValidator {
	t.Helper()
	repo := writeModuleDir(t, "x", "x.cue", policyChainSchemas)
	core := writeModuleDir(t, "pudl/core", "core.cue", `package core
#Item: {_pudl: {schema_type: "catchall"}, ...}
`)
	cv, err := NewChainValidator(repo, core)
	if err != nil {
		t.Fatal(err)
	}
	return cv
}

// TestValidateChainRejectsPolicy covers explicit schemas (import --schema and
// a model's populate.schema both validate through ValidateChain): naming a
// policy is an error that names the policy's type and points at policy
// evaluation, and types, subtypes included, stay assignable.
func TestValidateChainRejectsPolicy(t *testing.T) {
	cv := policyChainValidator(t)
	record := map[string]any{"name": "b", "encrypted": true, "versioned": true}

	_, err := cv.ValidateChain(record, "x.#BucketPCI")
	var policyErr *PolicySchemaError
	if !errors.As(err, &policyErr) {
		t.Fatalf("ValidateChain(policy) error = %v, want *PolicySchemaError", err)
	}
	if policyErr.Policy != "x.#BucketPCI" || policyErr.Type != "x.#Bucket" {
		t.Fatalf("error = %+v, want policy x.#BucketPCI of type x.#Bucket", policyErr)
	}
	for _, want := range []string{"x.#BucketPCI", "x.#Bucket", "pudl policy evaluate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	for _, typ := range []string{"x.#Bucket", "x.#BucketSpecial"} {
		if err := cv.CheckAssignable(typ); err != nil {
			t.Errorf("CheckAssignable(%s) = %v, want nil", typ, err)
		}
	}
	// Type-to-type fallback still works: a record that is not special falls
	// back to the subtype's base type.
	result, err := cv.ValidateChain(map[string]any{"name": "b"}, "x.#BucketSpecial")
	if err != nil {
		t.Fatal(err)
	}
	if result.AssignedSchema != "x.#Bucket" {
		t.Fatalf("assigned = %s, want fallback to x.#Bucket", result.AssignedSchema)
	}
}

// TestValidationChainSkipsPolicies covers the base_schema fallback chain: a
// policy ancestor is walked through but never offered as a fallback.
func TestValidationChainSkipsPolicies(t *testing.T) {
	cv := policyChainValidator(t)
	chain, err := cv.buildValidationChain("x.#Weird")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(chain, " "); got != "x.#Weird x.#Bucket pudl/core.#Item" {
		t.Fatalf("chain = %s, want the policy left out", got)
	}
}

func TestNearestType(t *testing.T) {
	cv := policyChainValidator(t)
	for schema, want := range map[string]string{
		"x.#BucketPCI":      "x.#Bucket",
		"x.#BucketHardened": "x.#Bucket",
		"x.#Bucket":         "x.#Bucket",
		"x.#BucketSpecial":  "x.#BucketSpecial",
		"x.#Missing":        "",
	} {
		if got := NearestType(schema, cv.GetSchemaMetadata); got != want {
			t.Errorf("NearestType(%s) = %q, want %q", schema, got, want)
		}
	}
}
