package cmd

import (
	"fmt"
	"strings"
)

// runFlags is the validated catalog-replay surface of a model run: the subset
// of the options the phase helpers consult.
type runFlags struct {
	fromCatalog  bool
	catalogScope string
}

// runOptions is the complete input of one model run within a check. It is
// built once from the check's options and passed explicitly, so no caller
// mutates package flag variables to drive a run.
type runOptions struct {
	runFlags

	// model is the registered model to run; empty for an ad-hoc --populate run.
	model         string
	populateSpec  string
	checkUpstream bool
}

// validateRunFlags enforces that a catalog replay names the records it
// replays. There is no way to infer which already-ingested records belong to a
// model: records carry whatever target their observer reported, so an unscoped
// replay would set-diff `desired` against every observation in the catalog —
// every model, every host, all time — and could report clean off another
// model's records.
func validateRunFlags(f runFlags) error {
	if f.fromCatalog && strings.TrimSpace(f.catalogScope) == "" {
		return fmt.Errorf("--from-catalog requires --catalog-scope (an observe snapshot ID, or the origin the records were ingested under)")
	}
	if !f.fromCatalog && strings.TrimSpace(f.catalogScope) != "" {
		return fmt.Errorf("--catalog-scope requires --from-catalog")
	}
	return nil
}
