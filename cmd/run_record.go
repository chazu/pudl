package cmd

import (
	"fmt"
	"time"

	"github.com/chazu/pudl/internal/database"
)

// resolveApplyBudget works out how many applies this run may make, from how many
// the model has already spent since it was last verified clean.
//
// The per-run `--max-iters` cap gives a halting guarantee inside one process and
// none at all across processes: a scheduler on `freshness.every`, or a
// crash-loop supervisor, grants a fresh cap on every restart, so a model that
// oscillates applies without bound. This is the durable half.
//
// Returns nil — no constraint, previous behaviour exactly — when the budget is
// disabled (`--max-applies 0`), when the run does not converge, or when the
// history cannot be read. A catalog that cannot answer must not silently refuse
// to apply; that failure mode is worse than the one being prevented.
func resolveApplyBudget(cat *runCatalog, model string, flags runFlags, live bool) *int {
	if !flags.converge || flags.dryRun || flags.maxApplies <= 0 {
		return nil
	}
	db, err := cat.optional()
	if err != nil {
		if live {
			fmt.Printf("warning: could not open catalog to read this model's apply history: %v\n", err)
			fmt.Println("         the durable apply budget is not enforced for this run")
		}
		return nil
	}
	spent, err := db.AppliesSinceLastClean(model)
	if err != nil {
		if live {
			fmt.Printf("warning: could not read this model's apply history: %v\n", err)
			fmt.Println("         the durable apply budget is not enforced for this run")
		}
		return nil
	}

	remaining := flags.maxApplies - spent
	if remaining < 0 {
		remaining = 0
	}
	if live && spent > 0 {
		fmt.Printf("apply budget: %d of %d remaining (spent since this model was last verified clean)\n",
			remaining, flags.maxApplies)
	}
	return &remaining
}

// runFinishState is what a completed run concluded, handed to the deferred
// finalizer so every exit path — including an early `return err` — records a
// terminal run row.
type runFinishState struct {
	verdict           string
	outcome           string
	needsVerification bool
	// note explains a verdict whose meaning is not evident from the value alone —
	// a scoped run whose model row diverges from its run row, or a verdict a
	// failed check demoted. More than one can apply, so they accumulate.
	note string
	// scoped records that `--only` restricted this run. A scoped `clean` covers a
	// subset, so it must not reset the model's durable apply budget.
	scoped bool
}

// addNote appends an explanation, keeping any already recorded. Two notes can
// both be true of one run (a scoped run whose check also failed), and dropping
// either would leave a verdict half-explained.
func (s *runFinishState) addNote(note string) {
	if note == "" {
		return
	}
	if s.note != "" {
		s.note += "; "
	}
	s.note += note
}

// startRunRecord opens the run's audit row before any phase runs, and reports any
// earlier run of this model that never finished. An unfinished row means a prior
// invocation died without recording a verdict, so the status that model currently
// carries predates it. Best-effort: auditing must not fail the run.
func startRunRecord(cat *runCatalog, runID, model, mode string, live bool) {
	db, err := cat.optional()
	if err != nil {
		if live {
			fmt.Printf("warning: could not open catalog to record the run: %v\n", err)
		}
		return
	}

	if stale, err := db.UnfinishedRuns(model); err == nil && len(stale) > 0 && live {
		fmt.Printf("warning: %d earlier run(s) of %q never finished (most recent: %s, started %s)\n",
			len(stale), model, stale[0].RunID, stale[0].StartedAt.Format(time.RFC3339))
		fmt.Println("         that model's recorded status predates those runs and may be stale")
	}
	if err := db.StartRun(runID, model, mode); err != nil && live {
		fmt.Printf("warning: could not record run start: %v\n", err)
	}
}

// finishRunRecord marks the run terminal. It runs from a defer so that an early
// error return is still a *recorded* termination — distinguishable from a process
// that died without saying anything, which leaves the row unfinished.
func finishRunRecord(cat *runCatalog, runID string, state runFinishState, runErr error, live bool) {
	// An open failure goes unreported here alone: startRunRecord borrowed the same
	// handle and already said so, and there is no row to finish anyway.
	db, err := cat.optional()
	if err != nil {
		return
	}

	if err := db.FinishRun(runID, runConclusion(state, runErr)); err != nil && live {
		fmt.Printf("warning: could not record run completion: %v\n", err)
	}
}

// runConclusion is the terminal run row for a run that concluded in state and
// ended with runErr.
func runConclusion(state runFinishState, runErr error) database.RunConclusion {
	// Both notes matter and neither supersedes the other: the scope note explains
	// the verdict, the error explains why the run ended.
	note := state.note
	if runErr != nil {
		if note != "" {
			note += "; "
		}
		note += runErr.Error()
	}
	completionStatus := database.RunStatusSucceeded
	if runErr != nil {
		completionStatus = database.RunStatusFailed
	}
	return database.RunConclusion{
		CompletionStatus:  completionStatus,
		Verdict:           state.verdict,
		Outcome:           state.outcome,
		NeedsVerification: state.needsVerification,
		Note:              note,
		Scoped:            state.scoped,
	}
}
