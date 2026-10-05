package inference

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/chazu/pudl/internal/validator"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInferenceTracePreservesCascadeAndReportsOnlyPaths(t *testing.T) {
	ctx := cuecontext.New()
	meta := map[string]validator.SchemaMetadata{"thing.#Thing": {SchemaType: "base", ResourceType: "thing"}, "pudl/core.#Item": {SchemaType: "catchall", ResourceType: "unknown"}}
	si := &SchemaInferrer{schemas: map[string]cue.Value{"thing.#Thing": ctx.CompileString(`{name: string, serial: int, ...}`), "pudl/core.#Item": ctx.CompileString(`{...}`)}, metadata: meta, graph: BuildInheritanceGraph(meta), sources: map[string]string{"thing.#Thing": "repo"}, shadowed: map[string][]string{"thing.#Thing": {"global"}}}
	for _, data := range []any{map[string]any{"name": "x", "serial": 1}, map[string]any{"name": 42, "secret": "do-not-copy-this"}} {
		hints := InferenceHints{DeclaredSchema: "thing"}
		plain, err := si.Infer(data, hints)
		require.NoError(t, err)
		traced, err := si.InferWithTrace(data, hints)
		require.NoError(t, err)
		require.Equal(t, plain.Schema, traced.Schema)
		require.Equal(t, plain.Confidence, traced.Confidence)
		require.Equal(t, plain.CascadePath, traced.CascadePath)
		require.Nil(t, plain.Trace)
		require.NotNil(t, traced.Trace)
		require.Contains(t, traced.Trace.ScoreKind, "not a calibrated probability")
		require.NotEmpty(t, traced.Trace.Attempts)
		attempt := traced.Trace.Attempts[0]
		require.Equal(t, "repo", attempt.SourcePath)
		require.Equal(t, []string{"global"}, attempt.ShadowedPaths)
		if traced.Trace.Fallback {
			require.Contains(t, attempt.FailurePaths, "name")
		}
	}
	empty := &SchemaInferrer{}
	result, err := empty.InferWithTrace(nil, InferenceHints{})
	require.NoError(t, err)
	require.True(t, result.Trace.Fallback)
}
