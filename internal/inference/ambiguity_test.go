package inference

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/chazu/pudl/internal/validator"
	"github.com/stretchr/testify/require"
)

func TestTraceReportsTiedAlternativesFromOtherFamilies(t *testing.T) {
	ctx := cuecontext.New()
	meta := map[string]validator.SchemaMetadata{
		"gcp.#Route":      {SchemaType: "base", ResourceType: "gcp.route", IdentityFields: []string{"project", "name"}},
		"gcp.#Router":     {SchemaType: "base", ResourceType: "gcp.router", IdentityFields: []string{"project", "name"}},
		"gcp.#RouteChild": {SchemaType: "policy", ResourceType: "gcp.route", BaseSchema: "gcp.#Route", IdentityFields: []string{"project", "name"}},
		"pudl/core.#Item": {SchemaType: "catchall", ResourceType: "unknown"},
	}
	open := ctx.CompileString(`{project: string, name: string, ...}`)
	si := &SchemaInferrer{
		schemas:  map[string]cue.Value{"gcp.#Route": open, "gcp.#Router": open, "gcp.#RouteChild": open, "pudl/core.#Item": ctx.CompileString(`{...}`)},
		metadata: meta, graph: BuildInheritanceGraph(meta), sources: map[string]string{}, shadowed: map[string][]string{},
	}
	data := map[string]any{"project": "p", "name": "r"}

	traced, err := si.InferWithTrace(data, InferenceHints{})
	require.NoError(t, err)
	require.Len(t, traced.Trace.Alternatives, 1, "one tie from another family; base/child ties are not ambiguity")

	plain, err := si.Infer(data, InferenceHints{})
	require.NoError(t, err)
	require.Equal(t, traced.Schema, plain.Schema, "reporting alternatives never changes the selection")
}
