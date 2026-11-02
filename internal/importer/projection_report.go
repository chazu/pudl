package importer

import (
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/policyfacts"
	"github.com/chazu/pudl/internal/projection"
)

// observeFacts makes an entry's projected facts and policy results its
// resource's current ones, in the transaction recording the import.
func observeFacts(tx *database.CatalogTx, projected *projection.Prepared, results *policyfacts.Prepared) error {
	if err := projection.Observe(tx, projected); err != nil {
		return err
	}
	_, err := policyfacts.Observe(tx, results, database.ProjectionObservation)
	return err
}

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
