package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// runFlags is the validated convergence surface for `pudl run`: the subset of
// the options the phase helpers (budget, verdict, plan rendering) consult.
type runFlags struct {
	converge bool
	only     []string
	dryRun   bool
	maxIters int
	// maxApplies is the durable cap: the applies this model may make in total
	// since it was last verified clean, across runs. 0 disables it.
	maxApplies   int
	fromCatalog  bool
	catalogScope string

	onlySet       bool
	dryRunSet     bool
	maxItersSet   bool
	maxAppliesSet bool
}

// runOptions is the complete input of one model run. It is built once — from
// the command's flags, from a run-set member's settings, or from a stored
// approval request — and passed explicitly, so no caller re-enters another
// command's handler or mutates package flag variables to drive a run.
type runOptions struct {
	runFlags

	// model is the registered model to run; empty for an ad-hoc --populate run.
	model         string
	muRoot        string
	populateSpec  string
	populateInput []string
	checkUpstream bool

	requireApproval bool
	// resumeID continues a pending approval under its original run identity.
	resumeID       string
	approvalStatus string

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

// defaultRunDeps is the production wiring: the real mu binary, no run set.
func defaultRunDeps() runDeps {
	return runDeps{mu: execMu{}}
}

// runOptionsFromFlags reads `pudl run`'s flags once. Changed() is consulted
// here and nowhere else; downstream code reads the *Set booleans.
func runOptionsFromFlags(cmd *cobra.Command, args []string) runOptions {
	opts := runOptions{
		runFlags: runFlags{
			converge:      runConverge,
			only:          runOnly,
			dryRun:        runDryRun,
			maxIters:      runMaxIters,
			maxApplies:    runMaxApplies,
			fromCatalog:   runFromCatalog,
			catalogScope:  runCatalogScope,
			onlySet:       cmd.Flags().Changed("only"),
			dryRunSet:     cmd.Flags().Changed("dry-run"),
			maxItersSet:   cmd.Flags().Changed("max-iters"),
			maxAppliesSet: cmd.Flags().Changed("max-applies"),
		},
		muRoot:               runMuRoot,
		populateSpec:         runPopulateSpec,
		populateInput:        runPopulateInput,
		checkUpstream:        runCheckUpstream,
		requireApproval:      runRequireApproval,
		maxObservationAge:    runMaxObservationAge,
		maxObservationAgeSet: cmd.Flags().Changed("max-observation-age"),
	}
	if len(args) > 0 {
		opts.model = args[0]
	}
	return opts
}

// memberRunOptions is the observe-only run a run-set member performs: default
// convergence settings, the set's mu root and observation-age policy.
func memberRunOptions(model string, set runSetOptions) runOptions {
	return runOptions{
		runFlags:             runFlags{maxIters: defaultRunMaxIters, maxApplies: defaultRunMaxApplies},
		model:                model,
		muRoot:               set.muRoot,
		maxObservationAge:    set.maxObservationAge,
		maxObservationAgeSet: set.maxObservationAgeSet,
	}
}

// resumedRunOptions rebuilds the converge run a stored approval request asked
// for, continuing it under the pending run's identity.
func resumedRunOptions(runID string, request approvalRequest) runOptions {
	return runOptions{
		runFlags: runFlags{
			converge: true, only: request.Only,
			maxIters: request.MaxIters, maxApplies: request.MaxApplies,
		},
		model:          request.Model,
		muRoot:         request.MuRoot,
		resumeID:       runID,
		approvalStatus: "approved",
	}
}

// validateRunFlags enforces the gate rules: convergence flags require --converge,
// and a catalog replay must name the records it replays. The first rule means a
// resource can't be named (or a plan dry-run requested) without explicitly opting
// into mutation. The second exists because there is no way to infer which
// already-ingested records belong to a model: records ingested by
// `pudl mu ingest-observe` carry whatever target their observer reported, so an
// unscoped replay would set-diff `desired` against every observation in the
// catalog — every model, every host, all time — and could report clean off
// another model's records.
func validateRunFlags(f runFlags) error {
	if f.fromCatalog && strings.TrimSpace(f.catalogScope) == "" {
		return fmt.Errorf("--from-catalog requires --catalog-scope (an observe snapshot ID, or the origin the records were ingested under)")
	}
	if !f.fromCatalog && strings.TrimSpace(f.catalogScope) != "" {
		return fmt.Errorf("--catalog-scope requires --from-catalog")
	}
	if f.converge {
		if f.maxIters < 1 {
			return fmt.Errorf("--max-iters must be >= 1")
		}
		if f.maxApplies < 0 {
			return fmt.Errorf("--max-applies must be >= 0 (0 disables the durable apply budget)")
		}
		return nil
	}
	switch {
	case f.onlySet:
		return fmt.Errorf("--only requires --converge")
	case f.dryRunSet:
		return fmt.Errorf("--dry-run requires --converge")
	case f.maxItersSet:
		return fmt.Errorf("--max-iters requires --converge")
	case f.maxAppliesSet:
		return fmt.Errorf("--max-applies requires --converge")
	}
	return nil
}
