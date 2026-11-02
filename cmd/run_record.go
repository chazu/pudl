package cmd

import (
	"fmt"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/proc"
)

// runFinishState is what a completed run concluded, handed to the deferred
// finalizer so every exit path — including an early `return err` — records a
// terminal run row.
type runFinishState struct {
	verdict string
	// note explains a verdict whose meaning is not evident from the value alone,
	// such as a verdict a failed check demoted.
	note string
}

// addNote appends an explanation, keeping any already recorded.
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
			fmt.Fprintf(errw(), "warning: could not open catalog to record the run: %v\n", err)
		}
		return
	}

	if stale, err := db.UnfinishedRuns(model); err == nil && len(stale) > 0 && live {
		fmt.Fprintf(errw(), "warning: %d earlier run(s) of %q never finished (most recent: %s, started %s)\n",
			len(stale), model, stale[0].RunID, stale[0].StartedAt.Format(time.RFC3339))
		fmt.Fprintln(outw(), "         that model's recorded status predates those runs and may be stale")
	}
	if err := db.StartRun(runID, model, mode); err != nil && live {
		fmt.Fprintf(errw(), "warning: could not record run start: %v\n", err)
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
		fmt.Fprintf(errw(), "warning: could not record run completion: %v\n", err)
	}
}

// failureStatus is the terminal completion status of a run that ended with err:
// `cancelled` when the operator interrupted it, `failed` otherwise. A timeout
// expiry is a failure, not a cancellation — nobody asked it to stop.
func failureStatus(err error) string {
	if proc.Cancelled(err) {
		return database.RunStatusCancelled
	}
	return database.RunStatusFailed
}

// runConclusion is the terminal run row for a run that concluded in state and
// ended with runErr.
func runConclusion(state runFinishState, runErr error) database.RunConclusion {
	// Both notes matter and neither supersedes the other: the state note
	// explains the verdict, the error explains why the run ended.
	note := state.note
	if runErr != nil {
		if note != "" {
			note += "; "
		}
		note += runErr.Error()
	}
	completionStatus := database.RunStatusSucceeded
	if runErr != nil {
		completionStatus = failureStatus(runErr)
	}
	return database.RunConclusion{
		CompletionStatus: completionStatus,
		Verdict:          state.verdict,
		Note:             note,
	}
}
