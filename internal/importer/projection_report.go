package importer

import "github.com/chazu/pudl/internal/projection"

// applyTally copies a projection tally into an import result.
func applyTally(result *ImportResult, tally *projection.Tally) {
	if tally == nil {
		return
	}
	if len(tally.Facts) > 0 {
		result.Facts = tally.Facts
	}
	result.FactWarnings = tally.Warnings()
}
