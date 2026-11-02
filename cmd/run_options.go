package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
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
	muRoot        string
	populateSpec  string
	populateInput []string
	checkUpstream bool

	maxObservationAge    time.Duration
	maxObservationAgeSet bool
}

// runDeps are the collaborators a run reaches outside its own options: the mu
// subprocess seam, and the run-set coordinator when the run is a set member.
type runDeps struct {
	mu muRunner
	// set is nil for a standalone run.
	set *runSetExecutionContext
}

// defaultRunDeps is the production wiring: the real mu binary bound to the
// invocation's context (and to --mu-timeout, when set), no run set.
func defaultRunDeps(ctx context.Context, muTimeout time.Duration) runDeps {
	return runDeps{mu: newExecMu(ctx, muTimeout)}
}

// runOptionsFromFlags reads `pudl run`'s flags once. Changed() is consulted
// here and nowhere else; downstream code reads the *Set booleans.
func runOptionsFromFlags(cmd *cobra.Command, args []string) runOptions {
	opts := runOptions{
		runFlags: runFlags{
			fromCatalog:  runFromCatalog,
			catalogScope: runCatalogScope,
		},
		muRoot:               runMuRoot,
		populateSpec:         runPopulateSpec,
		populateInput:        runPopulateInput,
		checkUpstream:        runCheckUpstream,
		maxObservationAge:    runMaxObservationAge,
		maxObservationAgeSet: cmd.Flags().Changed("max-observation-age"),
	}
	if len(args) > 0 {
		opts.model = args[0]
	}
	return opts
}

// memberRunOptions is the observe-only run a run-set member performs: the
// set's mu root and observation-age policy.
func memberRunOptions(model string, set runSetOptions) runOptions {
	return runOptions{
		model:                model,
		muRoot:               set.muRoot,
		maxObservationAge:    set.maxObservationAge,
		maxObservationAgeSet: set.maxObservationAgeSet,
	}
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
