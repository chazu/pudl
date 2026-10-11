package importer

import (
	"github.com/chazu/pudl/internal/inference"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSchemaExplanationDoesNotChangeAssignment(t *testing.T) {
	imp, _, _ := collectionFixture(t, "widget.json", `{"name":"w"}`)
	for _, manual := range []string{"", "pudl/core.#Item", "pudl/k8s.#Resource", "mu/missing@v1#Resource"} {
		t.Run(manual, func(t *testing.T) {
			plain, err := imp.assignSchema(map[string]any{"name": "w"}, ImportOptions{AllowSchemaFallback: true, ManualSchema: manual}, inference.InferenceHints{Format: "json"})
			require.NoError(t, err)
			traced, err := imp.assignSchema(map[string]any{"name": "w"}, ImportOptions{AllowSchemaFallback: true, ManualSchema: manual, Explain: true}, inference.InferenceHints{Format: "json"})
			require.NoError(t, err)
			require.Equal(t, plain.Schema, traced.Schema)
			require.Equal(t, plain.Confidence, traced.Confidence)
			require.Nil(t, plain.Trace)
			require.NotNil(t, traced.Trace)
			require.NotEmpty(t, traced.Reason)
			var info SchemaInfo
			var result ImportResult
			enrichAssignment(&info, &result, traced)
			require.Equal(t, traced.Reason, info.AssignmentReason)
			require.Equal(t, traced.Trace, result.Explanation)
			if manual == "mu/missing@v1#Resource" {
				require.Contains(t, traced.Reason, "unavailable")
			}
			if manual == "pudl/k8s.#Resource" {
				require.True(t, traced.Trace.Fallback)
				require.NotEmpty(t, traced.Trace.Attempts[0].FailurePaths)
			}
		})
	}
}
