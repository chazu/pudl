package inference

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeBrokenPackage(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "broken", "broken.cue")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("package broken\n\nthis is not valid cue {{{\n"), 0o644))
	future := time.Now().Add(time.Second)
	require.NoError(t, os.Chtimes(path, future, future))
}

func TestInfer_BrokenPackageStillMatchesTheGoodOnes(t *testing.T) {
	resetSchemaState(t)
	dir := sharedSchemaDir(t)
	writeBrokenPackage(t, dir)

	inferrer, err := Shared(dir)
	require.NoError(t, err)

	result, err := inferrer.Infer(map[string]any{"name": "widget"}, InferenceHints{})
	require.NoError(t, err)
	assert.Equal(t, "schemas.#Thing", result.Schema,
		"a broken sibling package must not turn every import into the catch-all")

	loadErrors := inferrer.LoadErrors()
	require.Len(t, loadErrors, 1)
	assert.Equal(t, "broken", loadErrors[0].Package)
}

func TestWarnLoadErrors(t *testing.T) {
	t.Run("clean schema tree writes nothing", func(t *testing.T) {
		resetSchemaState(t)
		dir := sharedSchemaDir(t)

		var out bytes.Buffer
		assert.False(t, WarnLoadErrors(&out, dir))
		assert.Empty(t, out.String())
	})

	t.Run("broken package writes one summary line", func(t *testing.T) {
		resetSchemaState(t)
		dir := sharedSchemaDir(t)
		writeBrokenPackage(t, dir)

		var out bytes.Buffer
		assert.True(t, WarnLoadErrors(&out, dir))
		assert.Equal(t,
			"warning: 1 schema load error; affected data falls back to the catch-all schema (run 'pudl doctor' for details)\n",
			out.String())
	})

	t.Run("cycle is reported", func(t *testing.T) {
		resetSchemaState(t)
		dir := sharedSchemaDir(t)
		writeSharedSchema(t, dir, "cycle.cue", `package schemas

#A: {
	_pudl: {schema_type: "base", resource_type: "a", base_schema: "schemas.#B", identity_fields: ["name"]}
	name: string
	...
}

#B: {
	_pudl: {schema_type: "base", resource_type: "b", base_schema: "schemas.#A", identity_fields: ["name"]}
	name: string
	...
}
`)

		var out bytes.Buffer
		assert.True(t, WarnLoadErrors(&out, dir))
		assert.Contains(t, out.String(), "1 base_schema cycle")
	})
}

func TestInheritanceGraph_CycleTerminates(t *testing.T) {
	resetSchemaState(t)
	dir := sharedSchemaDir(t)
	writeSharedSchema(t, dir, "cycle.cue", `package schemas

#A: {
	_pudl: {schema_type: "base", resource_type: "a", base_schema: "schemas.#B", identity_fields: ["name"]}
	name: string
	...
}

#B: {
	_pudl: {schema_type: "base", resource_type: "b", base_schema: "schemas.#A", identity_fields: ["name"]}
	name: string
	...
}
`)
	inferrer, err := NewSchemaInferrer(dir)
	require.NoError(t, err)

	graph := inferrer.GetInheritanceGraph()
	assert.Equal(t, []string{"schemas.#A", "schemas.#B"}, graph.GetCascadeChain("schemas.#A"),
		"the walk stops before revisiting a schema")
	assert.Equal(t, [][]string{{"schemas.#A", "schemas.#B", "schemas.#A"}}, inferrer.BaseSchemaCycles())
}
