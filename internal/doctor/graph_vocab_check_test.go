package doctor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphvocab"
)

func TestCheckGraphVocabulary(t *testing.T) {
	ok := CheckGraphVocabulary(&graphvocab.Vocabulary{EdgeTypes: []graphvocab.EdgeType{{Type: "in_subnet"}}}, nil)
	require.Equal(t, "ok", ok.Status)
	require.Contains(t, ok.Message, "1 edge types")

	warn := CheckGraphVocabulary(&graphvocab.Vocabulary{Problems: []string{"a", "b"}}, nil)
	require.Equal(t, "warning", warn.Status)
	require.Equal(t, "a\nb", warn.Details)

	failed := CheckGraphVocabulary(nil, errors.New("no rules"))
	require.Equal(t, "warning", failed.Status)
	require.Equal(t, "no rules", failed.Details)
}
