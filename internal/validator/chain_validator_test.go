package validator

import "testing"

// TestValidateChainAcrossSchemaPaths covers a chain whose schemas come from
// different search paths, and so from different CUE contexts: the intended
// schema fails in the first path and the catchall in the second path accepts.
func TestValidateChainAcrossSchemaPaths(t *testing.T) {
	repo := writeModuleDir(t, "thing", "thing.cue", `package thing

#Thing: {
	_pudl: {
		schema_type: "base"
		resource_type: "thing"
	}
	name: string
	...
}
`)
	global := writeModuleDir(t, "pudl/core", "core.cue", `package core

#Item: {
	_pudl: {
		schema_type: "catchall"
		resource_type: "unknown"
	}
	...
}
`)

	cv, err := NewChainValidator(repo, global)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := cv.ValidateChain(map[string]any{"name": "widget"}, "thing.#Thing")
	if err != nil {
		t.Fatal(err)
	}
	if ok.AssignedSchema != "thing.#Thing" {
		t.Fatalf("assigned schema = %q, want thing.#Thing", ok.AssignedSchema)
	}

	fallback, err := cv.ValidateChain(map[string]any{"name": 5}, "thing.#Thing")
	if err != nil {
		t.Fatal(err)
	}
	if fallback.AssignedSchema != "pudl/core.#Item" {
		t.Fatalf("assigned schema = %q, want pudl/core.#Item", fallback.AssignedSchema)
	}
}

func TestSchemaSetSourceShadowing(t *testing.T) {
	repo := writeModuleDir(t, "thing", "thing.cue", `package thing
#Thing: {_pudl: {schema_type: "base", resource_type: "thing"}, name: string, ...}
`)
	global := writeModuleDir(t, "thing", "thing.cue", `package thing
#Thing: {_pudl: {schema_type: "base", resource_type: "thing"}, name: int, ...}
`)
	set := LoadSchemaSet([]string{repo, global})
	if set.Sources["thing.#Thing"] != repo {
		t.Fatalf("source = %q", set.Sources["thing.#Thing"])
	}
	if len(set.Shadowed["thing.#Thing"]) != 1 || set.Shadowed["thing.#Thing"][0] != global {
		t.Fatalf("shadowed = %v", set.Shadowed)
	}
}
