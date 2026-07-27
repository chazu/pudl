package validator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// thingSchema is a definition carrying the _pudl block that makes it a tracked
// schema rather than a component.
const thingSchema = `package schemas

#Thing: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "test.thing"
		identity_fields: ["name"]
	}
	name: string
}
`

// schemaDir stages a minimal CUE module with one registered schema.
func schemaDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"),
		[]byte("module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	writeSchema(t, dir, filepath.Join("schemas", "thing.cue"), thingSchema)
	return dir
}

func writeSchema(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	// Timestamps are part of the fingerprint; make sure a rewrite in the same
	// test is distinguishable on filesystems with coarse resolution.
	future := time.Now().Add(time.Duration(len(body)) * time.Millisecond)
	require.NoError(t, os.Chtimes(path, future, future))
}

// loads runs fn and reports how many real compiles it caused.
func loads(t *testing.T, fn func()) int64 {
	t.Helper()
	before := ModuleLoadCount()
	fn()
	return ModuleLoadCount() - before
}

func TestModuleCache_SecondLoadIsServedFromTheMemo(t *testing.T) {
	dir := schemaDir(t)
	loader := NewCUEModuleLoader(dir)

	first := loads(t, func() {
		_, err := loader.LoadAllModules()
		require.NoError(t, err)
	})
	assert.Equal(t, int64(1), first)

	second := loads(t, func() {
		_, err := loader.LoadAllModules()
		require.NoError(t, err)
	})
	assert.Equal(t, int64(0), second, "unchanged files compile once")
}

func TestModuleCache_ChangedFileInvalidates(t *testing.T) {
	// The correctness case, not just a staleness one: `pudl schema new` writes a
	// schema and then infers against it inside one invocation.
	dir := schemaDir(t)
	loader := NewCUEModuleLoader(dir)

	_, err := loader.LoadAllModules()
	require.NoError(t, err)

	writeSchema(t, dir, filepath.Join("schemas", "thing.cue"), thingSchema+"\n#Extra: {\n\tid: string\n}\n")

	recompiled := loads(t, func() {
		modules, err := loader.LoadAllModules()
		require.NoError(t, err)
		require.NotEmpty(t, modules)
	})
	assert.Equal(t, int64(1), recompiled, "a changed file recompiles")
}

func TestModuleCache_AddedAndRemovedFilesInvalidate(t *testing.T) {
	dir := schemaDir(t)
	loader := NewCUEModuleLoader(dir)
	_, err := loader.LoadAllModules()
	require.NoError(t, err)

	writeSchema(t, dir, filepath.Join("schemas", "other.cue"), "package schemas\n\n#Other: {\n\tid: string\n}\n")
	added := loads(t, func() {
		_, err := loader.LoadAllModules()
		require.NoError(t, err)
	})
	assert.Equal(t, int64(1), added, "an added file recompiles")

	require.NoError(t, os.Remove(filepath.Join(dir, "schemas", "other.cue")))
	removed := loads(t, func() {
		_, err := loader.LoadAllModules()
		require.NoError(t, err)
	})
	assert.Equal(t, int64(1), removed, "a removed file recompiles")
}

func TestSharedLoader_TwoCallersShareOneCompile(t *testing.T) {
	ResetSharedLoaders()
	t.Cleanup(ResetSharedLoaders)
	dir := schemaDir(t)

	total := loads(t, func() {
		_, err := SharedLoader(dir).LoadAllModules()
		require.NoError(t, err)
		_, err = SharedLoader(dir).LoadAllModules()
		require.NoError(t, err)
	})
	assert.Equal(t, int64(1), total)
	assert.Same(t, SharedLoader(dir), SharedLoader(dir))
}

func TestSharedLoader_DistinctPathsAreDistinctLoaders(t *testing.T) {
	ResetSharedLoaders()
	t.Cleanup(ResetSharedLoaders)

	a, b := schemaDir(t), schemaDir(t)
	assert.NotSame(t, SharedLoader(a), SharedLoader(b))
}

func TestSharedLoader_ResolvesRelativeAndAbsoluteToTheSameLoader(t *testing.T) {
	ResetSharedLoaders()
	t.Cleanup(ResetSharedLoaders)

	dir := schemaDir(t)
	absolute, err := filepath.Abs(dir)
	require.NoError(t, err)
	assert.Same(t, SharedLoader(dir), SharedLoader(absolute))
}

func TestModuleCache_CallerMutationDoesNotPoisonTheCache(t *testing.T) {
	dir := schemaDir(t)
	loader := NewCUEModuleLoader(dir)

	first, err := loader.LoadAllModules()
	require.NoError(t, err)
	require.NotEmpty(t, first)

	// Vandalize what we were handed, remembering what we destroyed.
	var vandalizedModule, vandalizedSchema string
	for name, module := range first {
		vandalizedModule = name
		for schema := range module.Schemas {
			vandalizedSchema = schema
			delete(module.Schemas, schema)
			break
		}
		module.Metadata["injected"] = SchemaMetadata{}
		delete(first, name)
		break
	}
	require.NotEmpty(t, vandalizedSchema, "the fixture must define at least one schema")

	second, err := loader.LoadAllModules()
	require.NoError(t, err)
	module, ok := second[vandalizedModule]
	require.True(t, ok, "the module deleted from the first caller's copy is still there")
	_, injected := module.Metadata["injected"]
	assert.False(t, injected, "the next caller must not see the previous caller's mutation")
	_, kept := module.Schemas[vandalizedSchema]
	assert.True(t, kept, "the schema deleted from the first caller's copy is still there")
}

func TestModuleCache_FailedLoadIsNotCached(t *testing.T) {
	// A failed load is exactly when the caller may fix the problem and retry, so
	// caching the failure would hide the fix until the next process.
	dir := schemaDir(t)
	writeSchema(t, dir, filepath.Join("schemas", "broken.cue"), "package schemas\n\nthis is not valid cue {{{\n")
	loader := NewCUEModuleLoader(dir)

	if _, err := loader.LoadAllModules(); err == nil {
		t.Skip("the loader tolerates this malformed file; nothing to assert about caching a failure")
	}

	retried := loads(t, func() { _, _ = loader.LoadAllModules() })
	assert.Equal(t, int64(1), retried, "a failure is retried, not served from the memo")
}
