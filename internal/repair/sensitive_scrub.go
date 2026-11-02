package repair

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/scrub"
)

// sensitiveScrubRemedy is said whenever the scrub finds or redacts plaintext.
const sensitiveScrubRemedy = "Fact history and exported bundles keep the plaintext, and old IDs remain in redirects: rotate every credential this redacts"

// sensitiveScrub redacts plaintext stored before a schema declared the field
// in _pudl.sensitive_fields, re-deriving the IDs of entries whose content
// changes (see internal/scrub). It is opt-in: `--fix` applies it only when
// `--only sensitive-scrub` names it.
var sensitiveScrub = Repair{
	Name:      "sensitive-scrub",
	Condition: "stored plaintext at paths a schema now declares in _pudl.sensitive_fields",
	OptIn:     true,
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		plan, err := scrubber(env).Plan(ctx)
		if err != nil {
			return Result{}, err
		}
		return scrubResult(&scrub.Outcome{Plan: plan}, false), nil
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		outcome, err := scrubber(env).Apply(ctx, dryRun)
		if err != nil {
			return Result{}, err
		}
		return scrubResult(outcome, !dryRun), nil
	},
}

func scrubber(env *Env) *scrub.Scrubber {
	return &scrub.Scrubber{DB: env.DB, Inferrer: env.Inferrer, Projections: env.Projections,
		Scope: scrub.Scope{EntryID: env.Options.EntryID, Schema: env.Options.Schema, Origin: env.Options.Origin}}
}

// scrubResult reports subjects, IDs and paths, never values.
func scrubResult(outcome *scrub.Outcome, applied bool) Result {
	plan := outcome.Plan
	result := Result{Pending: plan.Pending(), Errors: append([]string{}, plan.Errors...), Details: map[string]any{
		"subjects": plan.Subjects(), "unsupported": plan.Unsupported, "superseded_files": plan.Leftovers,
		"compaction_pending": plan.Uncompacted, "short_values_skipped": plan.ShortValuesSkipped(), "scrub_id": outcome.ScrubID,
	}}
	for _, s := range plan.Subjects() {
		to := strings.Join(s.Paths, ", ") + " redacted"
		if s.NewID != "" {
			to += "; ID " + idgen.HashToProquint(s.NewID)
			if s.Merged {
				to += " (merged)"
			}
		}
		result.Changes = append(result.Changes, Change{Subject: s.Kind + " " + subjectName(s), From: "plaintext", To: to})
	}
	for _, path := range plan.Leftovers {
		result.Changes = append(result.Changes, Change{Subject: path, From: "superseded", To: "unlinked"})
	}
	for _, u := range plan.Unsupported {
		result.Errors = append(result.Errors, fmt.Sprintf("entry %s [%s]: plaintext at %s in a non-JSON or compressed payload, which the scrub cannot rewrite; delete the entry and re-import it as JSON",
			idgen.HashToProquint(u.ID), u.Schema, strings.Join(u.Paths, ", ")))
	}
	if applied {
		result.Changed = outcome.Changed
		result.Errors = append(result.Errors, outcome.Errors...)
		result.Failed = len(plan.Unsupported) + len(plan.Errors) + len(outcome.Errors)
	}
	switch {
	case result.Pending == 0:
		result.Message = "No stored plaintext at sensitive paths"
	case applied && outcome.CompactionPending:
		result.Message = fmt.Sprintf("Redacted %d artifacts; compaction pending (another pudl process holds the catalog), rerun to finish. %s", outcome.Changed, sensitiveScrubRemedy)
	case applied:
		result.Message = fmt.Sprintf("Redacted %d artifacts and compacted the catalog. %s", outcome.Changed, sensitiveScrubRemedy)
	default:
		result.Message = fmt.Sprintf("%d artifacts hold plaintext at sensitive paths or are left from an interrupted scrub (opt-in: name it with --only sensitive-scrub). %s",
			result.Pending, sensitiveScrubRemedy)
	}
	if n := plan.ShortValuesSkipped(); n > 0 {
		result.Message += fmt.Sprintf(" %d values shorter than 8 bytes were not searched for in reports and other files.", n)
	}
	return result
}

func subjectName(s scrub.Subject) string {
	if strings.HasPrefix(s.Kind, "run_") {
		return s.ID
	}
	return idgen.HashToProquint(s.ID)
}
