package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/chazu/pudl/internal/inference"
	"strings"

	"github.com/chazu/pudl/internal/importer"
)

// Import outcome statuses reported under --json.
const (
	importStatusImported = "imported"
	importStatusSkipped  = "skipped"
	importStatusFailed   = "failed"
	importStatusDryRun   = "dry-run"
)

// importOutcome is one file's entry in `pudl import --json`: the import result
// fields, plus a status and, for a failure, the error.
type importOutcome struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	*importer.ImportResult
}

// newImportOutcome describes the result of importing sourcePath.
func newImportOutcome(sourcePath string, result *importer.ImportResult, err error) importOutcome {
	switch {
	case err != nil:
		return importOutcome{
			Status:       importStatusFailed,
			Error:        err.Error(),
			ImportResult: &importer.ImportResult{SourcePath: sourcePath, RequestedSchema: importSchema},
		}
	case result.DryRun:
		return importOutcome{Status: importStatusDryRun, ImportResult: result}
	case result.Skipped:
		return importOutcome{Status: importStatusSkipped, ImportResult: result}
	default:
		return importOutcome{Status: importStatusImported, ImportResult: result}
	}
}

// writeImportJSON writes outcomes as one JSON array on the result stream.
func writeImportJSON(outcomes []importOutcome) error {
	if outcomes == nil {
		outcomes = []importOutcome{}
	}
	encoder := json.NewEncoder(outw())
	encoder.SetIndent("", "  ")
	return encoder.Encode(outcomes)
}

func displayImportExplanation(result *importer.ImportResult) {
	if result.Explanation != nil {
		renderImportTrace(result.Explanation)
	}
	for _, item := range result.ItemExplanations {
		fmt.Fprintf(outw(), "   Item %d classification:\n", item.Index)
		renderImportTrace(item.Trace)
	}
	if result.ExplanationsTruncated {
		fmt.Fprintln(outw(), "   Further item explanations omitted; individual metadata retains original reasons.")
	}
	if result.Skipped && result.Explanation == nil && len(result.ItemExplanations) == 0 {
		fmt.Fprintln(outw(), "   Original classification trace unavailable; import was deduplicated without reassessment.")
	}
}
func renderImportTrace(trace *inference.InferenceTrace) {
	fmt.Fprintf(outw(), "   Classification: %s (%s)\n", trace.Selected, trace.Reason)
	fmt.Fprintf(outw(), "   Scores: %s\n", trace.ScoreKind)
	for _, attempt := range trace.Attempts {
		fmt.Fprintf(outw(), "      %s score=%.2f matched=%t: %s\n", attempt.Schema, attempt.Score, attempt.Matched, attempt.Reason)
		if attempt.SourcePath != "" {
			fmt.Fprintf(outw(), "         source: %s\n", attempt.SourcePath)
		}
		if len(attempt.ShadowedPaths) > 0 {
			fmt.Fprintf(outw(), "         shadows: %s\n", strings.Join(attempt.ShadowedPaths, ", "))
		}
		if len(attempt.FailurePaths) > 0 {
			fmt.Fprintf(outw(), "         failed fields: %s\n", strings.Join(attempt.FailurePaths, ", "))
		}
	}
	if trace.AttemptsTruncated {
		fmt.Fprintln(outw(), "      Further candidate attempts omitted.")
	}
	for _, path := range trace.LoadFailures {
		fmt.Fprintf(outw(), "      schema load failed: %s\n", path)
	}
}
