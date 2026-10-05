package cmd

import (
	"encoding/json"

	"github.com/chazu/pudl/internal/importer"
)

// Import outcome statuses reported under --json.
const (
	importStatusImported = "imported"
	importStatusSkipped  = "skipped"
	importStatusFailed   = "failed"
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
			ImportResult: &importer.ImportResult{SourcePath: sourcePath},
		}
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
