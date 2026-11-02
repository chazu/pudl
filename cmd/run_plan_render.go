package cmd

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// renderRunPlan renders the phase plan for a model. Every run is observe-only.
func renderRunPlan(m *systemmodel.SystemModel) string {
	var b strings.Builder
	fmt.Fprintf(&b, "model:    %s\n", m.Name)
	fmt.Fprintf(&b, "populate: %s (%s)\n", m.Populate.Kind(), populateRef(m.Populate))
	fmt.Fprintf(&b, "checks:   %d\n", len(m.Checks))
	fmt.Fprintf(&b, "mode:     %s\n", "observe-only")

	fmt.Fprintln(&b, "\nphases:")
	fmt.Fprintln(&b, "  1. populate -> ingest (Accumulate)")
	fmt.Fprintln(&b, "  2. drift             (Unify)")
	fmt.Fprintln(&b, "  3. checks            (flag)")
	fmt.Fprintln(&b, "  4. report")
	return b.String()
}

// populateRef returns a short description of the populate arm.
func populateRef(p systemmodel.Populate) string {
	if p.Kind() == systemmodel.KindNone {
		return "checks-only"
	}
	return fmt.Sprintf("%d runs", len(p.Runs))
}
