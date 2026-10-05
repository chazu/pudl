package validator

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const brokenPackage = "package broken\n\nthis is not valid cue {{{\n"

func TestLoadModules_BrokenPackageDoesNotHideTheOthers(t *testing.T) {
	ResetSharedLoaders()
	t.Cleanup(ResetSharedLoaders)
	dir := schemaDir(t)
	writeSchema(t, dir, filepath.Join("broken", "broken.cue"), brokenPackage)
	loader := NewCUEModuleLoader(dir)

	modules, loadErrs, err := loader.LoadModules()
	require.NoError(t, err, "one broken package is not a failure of the whole path")
	require.Contains(t, modules, "schemas", "the good package still loads")
	require.Len(t, loadErrs, 1)
	assert.Equal(t, "broken", loadErrs[0].Package)
	assert.Equal(t, dir, loadErrs[0].Path)
	assert.Contains(t, loadErrs[0].Error(), "schema package broken")

	_, strictErr := loader.LoadAllModules()
	assert.Error(t, strictErr, "the strict load still fails, for callers that must not see a partial tree")

	set := LoadSchemaSet([]string{dir})
	assert.Contains(t, set.Schemas, "schemas.#Thing")
	require.Len(t, set.Errors, 1)
	assert.Equal(t, "broken", set.Errors[0].Package)
}

func TestLoadModules_SamePackageNameInTwoDirectories(t *testing.T) {
	dir := schemaDir(t)
	writeSchema(t, dir, filepath.Join("first", "k8s", "pod.cue"), `package k8s

#Pod: {
	_pudl: {schema_type: "base", resource_type: "first.pod", identity_fields: ["name"]}
	name: string
}
`)
	writeSchema(t, dir, filepath.Join("second", "k8s", "svc.cue"), `package k8s

#Service: {
	_pudl: {schema_type: "base", resource_type: "second.service", identity_fields: ["name"]}
	name: string
}
`)
	loader := NewCUEModuleLoader(dir)

	modules, err := loader.LoadAllModules()
	require.NoError(t, err)
	assert.Contains(t, modules, "first/k8s")
	assert.Contains(t, modules, "second/k8s")

	schemas := loader.GetAllSchemas(modules)
	assert.Contains(t, schemas, "first/k8s.#Pod", "neither package may overwrite the other")
	assert.Contains(t, schemas, "second/k8s.#Service", "neither package may overwrite the other")
}

const cyclicSchemas = `package cyc

#A: {
	_pudl: {schema_type: "base", resource_type: "cyc.a", base_schema: "cyc.#B", identity_fields: ["name"]}
	name: string
	...
}

#B: {
	_pudl: {schema_type: "base", resource_type: "cyc.b", base_schema: "cyc.#A", identity_fields: ["name"]}
	name: string
	...
}
`

func TestValidateChain_BaseSchemaCycleIsAnError(t *testing.T) {
	ResetSharedLoaders()
	t.Cleanup(ResetSharedLoaders)
	dir := schemaDir(t)
	writeSchema(t, dir, filepath.Join("cyc", "cyc.cue"), cyclicSchemas)

	cv, err := NewChainValidator(dir)
	require.NoError(t, err)
	want := []string{"cyc.#A", "cyc.#B", "cyc.#A"}
	assert.Equal(t, [][]string{want}, cv.BaseSchemaCycles())

	type outcome struct {
		result *ValidationResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := cv.ValidateChain(map[string]any{"name": "x"}, "cyc.#A")
		done <- outcome{result, err}
	}()

	select {
	case got := <-done:
		require.Error(t, got.err)
		assert.Nil(t, got.result)
		assert.Equal(t, "base_schema cycle: "+strings.Join(want, " → "), got.err.Error())
	case <-time.After(10 * time.Second):
		t.Fatal("ValidateChain did not return on a base_schema cycle")
	}
}

func TestFindBaseSchemaCycles(t *testing.T) {
	metadata := map[string]SchemaMetadata{
		"x.#Leaf": {BaseSchema: "x.#B"},
		"x.#B":    {BaseSchema: "x.#C"},
		"x.#C":    {BaseSchema: "x.#B"},
		"x.#Self": {BaseSchema: "x.#Self"},
		"x.#Root": {},
		"x.#Kid":  {BaseSchema: "x.#Root"},
	}

	assert.Equal(t, [][]string{
		{"x.#B", "x.#C", "x.#B"},
		{"x.#Self", "x.#Self"},
	}, FindBaseSchemaCycles(metadata), "each cycle is reported once, starting at its smallest member")
}

func TestNewChainValidator_BaseSchemaMayLiveInAnotherPath(t *testing.T) {
	ResetSharedLoaders()
	t.Cleanup(ResetSharedLoaders)
	global := schemaDir(t)
	writeSchema(t, global, filepath.Join("base", "root.cue"), `package base

#Root: {
	_pudl: {schema_type: "base", resource_type: "base.root", identity_fields: ["name"]}
	name: string
	...
}
`)
	repo := schemaDir(t)
	writeSchema(t, repo, filepath.Join("schemas", "child.cue"), `package schemas

#Child: {
	_pudl: {schema_type: "policy", resource_type: "base.root", base_schema: "base.#Root", identity_fields: ["name"]}
	name: string
	...
}
`)

	cv, err := NewChainValidator(repo, global)
	require.NoError(t, err)
	assert.True(t, cv.HasSchema("schemas.#Child"),
		"a repository schema extending a global base must not drop the repository path")
	assert.Empty(t, cv.LoadErrors())
}

func TestLoadSchemaSet_ReportsMissingBaseSchema(t *testing.T) {
	ResetSharedLoaders()
	t.Cleanup(ResetSharedLoaders)
	dir := schemaDir(t)
	writeSchema(t, dir, filepath.Join("orphan", "orphan.cue"), `package orphan

#Orphan: {
	_pudl: {schema_type: "policy", resource_type: "orphan", base_schema: "nowhere.#Gone", identity_fields: ["name"]}
	name: string
}
`)

	set := LoadSchemaSet([]string{dir})
	assert.Contains(t, set.Schemas, "orphan.#Orphan", "the schema still loads")
	require.Len(t, set.Errors, 1)
	assert.Contains(t, set.Errors[0].Error(), "references non-existent base schema nowhere.#Gone")
}

func TestLoadSchemaSet_MissingPathIsNotAnError(t *testing.T) {
	set := LoadSchemaSet([]string{filepath.Join(t.TempDir(), "absent")})
	assert.Empty(t, set.Errors)
	assert.Empty(t, set.Schemas)
}
