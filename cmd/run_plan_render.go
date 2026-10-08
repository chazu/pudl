package cmd

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/systemmodel"
)

// printModelDrift renders a model-level drift verdict.
func printModelDrift(r ModelDriftResult) {
	if r.Clean {
		fmt.Fprintln(outw(), "drift: ∅ (clean — all desired resources exist and match)")
		return
	}
	fmt.Fprintf(outw(), "drift: %d resource(s)\n", len(r.Drifted))
	for _, d := range r.Drifted {
		if d.Diff != "" {
			fmt.Fprintf(outw(), "  ~ %s (%s): %s\n", d.Resource, d.Reason, d.Diff)
		} else {
			fmt.Fprintf(outw(), "  ~ %s (%s)\n", d.Resource, d.Reason)
		}
		for _, f := range d.Fields {
			fmt.Fprintf(outw(), "      %s\n", f.Detail())
		}
	}
}

// buildRunPlan renders the resolved phase plan for a model under the given flags.
// Observe-only when --converge is absent (or the model declares no converge arm);
// the converge loop otherwise.
func buildRunPlan(m *systemmodel.SystemModel, f runFlags) string {
	plan, err := acute.NewRunPlan(m, acute.RunRequest{
		Converge: f.converge,
		Only:     f.only,
		DryRun:   f.dryRun,
		MaxIters: f.maxIters,
	})
	if err != nil {
		return fmt.Sprintf("error: %v\n", err)
	}
	return renderRunPlan(plan)
}

func renderRunPlan(plan *acute.RunPlan) string {
	m := plan.Effective
	f := runFlags{
		converge: plan.Request.Converge,
		only:     plan.Request.Only,
		dryRun:   plan.Request.DryRun,
		maxIters: plan.Request.MaxIters,
	}
	var b strings.Builder
	fmt.Fprintf(&b, "model:    %s\n", m.Name)
	fmt.Fprintf(&b, "populate: %s (%s)\n", m.Populate.Kind(), populateRef(m.Populate))
	fmt.Fprintf(&b, "checks:   %d\n", len(m.Checks))

	mode := "observe-only"
	if f.converge {
		if !m.Convergent() {
			mode = "observe-only (model declares no converge arm; --converge is a no-op)"
		} else {
			mode = fmt.Sprintf("converge via %q (max-iters %d)", m.Converge.Plugin, f.maxIters)
			if f.dryRun {
				mode += ", dry-run (plan only, no execute, no status writes)"
			}
			if len(f.only) > 0 {
				mode += fmt.Sprintf(", only: %s", strings.Join(f.only, ","))
			}
		}
	}
	fmt.Fprintf(&b, "mode:     %s\n", mode)

	fmt.Fprintln(&b, "\nphases:")
	fmt.Fprintln(&b, "  1. populate -> ingest (Accumulate)")
	fmt.Fprintln(&b, "  2. drift             (Unify)")
	fmt.Fprintln(&b, "  3. checks            (flag)")
	fmt.Fprintln(&b, "  4. report")
	if f.converge && m.Convergent() {
		fmt.Fprintln(&b, "  loop: drift==∅ -> clean | cap -> failed | else converge->execute->re-observe")
	}
	return b.String()
}

// populateRef returns a short identifier for the populate arm (plugin name or
// ewe source).
func populateRef(p systemmodel.Populate) string {
	switch p.Kind() {
	case systemmodel.KindEweTarget:
		return p.EweSource
	case systemmodel.KindCommand:
		return fmt.Sprintf("%d runs", len(p.Runs))
	case systemmodel.KindNone:
		return "checks-only"
	}
	return p.Plugin
}
