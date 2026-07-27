package inference

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sharedThingSchema = `package schemas

#Thing: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "test.thing"
		identity_fields: ["name"]
	}
	name: string
}
`

func sharedSchemaDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"),
		[]byte("module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	writeSharedSchema(t, dir, "thing.cue", sharedThingSchema)
	return dir
}

func writeSharedSchema(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, "schemas", name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	future := time.Now().Add(time.Duration(len(body)) * time.Millisecond)
	require.NoError(t, os.Chtimes(path, future, future))
}

func resetSchemaState(t *testing.T) {
	t.Helper()
	ResetShared()
	validator.ResetSharedLoaders()
	t.Cleanup(func() {
		ResetShared()
		validator.ResetSharedLoaders()
	})
}

func TestShared_SameInferrerForUnchangedPaths(t *testing.T) {
	resetSchemaState(t)
	dir := sharedSchemaDir(t)

	first, err := Shared(dir)
	require.NoError(t, err)
	second, err := Shared(dir)
	require.NoError(t, err)

	assert.Same(t, first, second,
		"the inheritance graph and merged metadata are built once, not per call")
}

func TestShared_ChangedSchemaRebuilds(t *testing.T) {
	// The correctness case: a command that writes a schema and then infers
	// against it inside one invocation must see what it just wrote.
	resetSchemaState(t)
	dir := sharedSchemaDir(t)

	first, err := Shared(dir)
	require.NoError(t, err)

	writeSharedSchema(t, dir, "other.cue", `package schemas

#Other: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "test.other"
		identity_fields: ["id"]
	}
	id: string
}
`)

	second, err := Shared(dir)
	require.NoError(t, err)
	assert.NotSame(t, first, second, "a new schema rebuilds the inferrer")

	_, ok := second.GetSchemaMetadata("schemas.#Other")
	assert.True(t, ok, "and the rebuilt inferrer sees the new schema")
}

func TestShared_DistinctPathListsAreDistinctInferrers(t *testing.T) {
	resetSchemaState(t)
	a, b := sharedSchemaDir(t), sharedSchemaDir(t)

	first, err := Shared(a)
	require.NoError(t, err)
	second, err := Shared(b)
	require.NoError(t, err)
	assert.NotSame(t, first, second)

	// Order is part of the key: shadowing is first-found-wins, so [a b] and
	// [b a] are different inferrers, not the same one.
	ab, err := Shared(a, b)
	require.NoError(t, err)
	ba, err := Shared(b, a)
	require.NoError(t, err)
	assert.NotSame(t, ab, ba)
}

func TestShared_CompilesEachPathOnce(t *testing.T) {
	resetSchemaState(t)
	dir := sharedSchemaDir(t)

	before := validator.ModuleLoadCount()
	for i := 0; i < 3; i++ {
		_, err := Shared(dir)
		require.NoError(t, err)
	}
	assert.Equal(t, int64(1), validator.ModuleLoadCount()-before,
		"three callers, one CUE compile")
}

func TestShared_NewSchemaInferrerStaysUnshared(t *testing.T) {
	// The escape hatch: a caller that needs an isolated inferrer still gets one.
	resetSchemaState(t)
	dir := sharedSchemaDir(t)

	first, err := NewSchemaInferrer(dir)
	require.NoError(t, err)
	second, err := NewSchemaInferrer(dir)
	require.NoError(t, err)
	assert.NotSame(t, first, second)
}
