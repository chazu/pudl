package doctor

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/graphvocab"
)

// CheckGraphVocabulary reports what pudl ui cannot draw as intended: types
// with no display info, undeclared edge types, unknown categories and
// conflicting declarations. The UI still renders such items (as unknown or
// "other"), so problems are warnings.
func CheckGraphVocabulary(v *graphvocab.Vocabulary, err error) *CheckResult {
	if err != nil {
		return &CheckResult{
			Status:  "warning",
			Message: "Graph vocabulary could not be built",
			Details: err.Error(),
			Fix:     "Run 'pudl graph vocab' for details",
		}
	}
	if len(v.Problems) == 0 {
		return &CheckResult{
			Status:  "ok",
			Message: fmt.Sprintf("Graph vocabulary is complete: %d resource types, %d edge types", len(v.ResourceTypes), len(v.EdgeTypes)),
		}
	}
	return &CheckResult{
		Status:  "warning",
		Message: fmt.Sprintf("%d graph vocabulary problem(s); pudl ui draws those types as unknown", len(v.Problems)),
		Details: strings.Join(v.Problems, "\n"),
		Fix:     "Add display info and roles to the schema's _pudl graph block, or graph_edge_type facts to a rule file (docs/schema-authoring.md, \"Make a type appear in pudl ui\"); 'pudl graph vocab' lists the vocabulary",
	}
}
