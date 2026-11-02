package cmd

import (
	"fmt"
	"strings"
)

// runFlags is the validated catalog-replay surface for `pudl run`: the subset
// of the options the phase helpers consult.
type runFlags struct {
	fromCatalog  bool
	catalogScope string
}

// runOptions is the complete input of one model run. It is built once — from
// the command's flags or from a run-set member's settings — and passed
// explicitly, so no caller re-enters another command's handler or mutates
// package flag variables to drive a run.
type runOptions struct {
	runFlags

	// model is the registered model to run; empty for an ad-hoc --populate run.
	model         string
	populateSpec  string
	checkUpstream bool
}

// runDeps are the collaborators a run reaches outside its own options: the
// run-set coordinator when the run is a set member.
type runDeps struct {
	// set is nil for a standalone run.
	set *runSetExecutionContext
}

// runOptionsFromFlags reads `pudl run`'s flags once.
func runOptionsFromFlags(args []string) runOptions {
	opts := runOptions{
		runFlags: runFlags{
			fromCatalog:  runFromCatalog,
			catalogScope: runCatalogScope,
		},
		populateSpec:  runPopulateSpec,
		checkUpstream: runCheckUpstream,
	}
	if len(args) > 0 {
		opts.model = args[0]
	}
	return opts
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
