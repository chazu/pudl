package cmd

import "github.com/chazu/pudl/internal/checkreport"

// applyRunError copies a phase error into the report before it is rendered or
// persisted. Reports are persisted before RunE returns, so relying only on the
// deferred named-return finalizer would lose the error details.
func applyRunError(report *checkreport.ModelReport, err error) {
	if report == nil || err == nil {
		return
	}
	report.OK = false
	report.Error = err.Error()
	report.CompletionStatus = failureStatus(err)
}
