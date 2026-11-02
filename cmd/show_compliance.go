package cmd

import (
	"fmt"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/lister"
	"github.com/chazu/pudl/internal/policy"
	"github.com/chazu/pudl/internal/policyfacts"
	"github.com/chazu/pudl/internal/validator"
)

// complianceResult is one applicable policy's stored result for the entry's
// resource. Status is not_evaluated when no result is stored.
type complianceResult struct {
	policyfacts.Stored
	// Provisional is true when the entry's type is a draft.
	Provisional bool `json:"provisional"`
	Stale       bool `json:"stale"`
}

// entryCompliance reads the stored results of every policy applicable to the
// entry's type. A failure is a warning: the rest of `pudl show` still prints.
func entryCompliance(entry lister.ListEntry) []complianceResult {
	results, err := loadEntryCompliance(entry)
	if err != nil {
		fmt.Fprintf(errw(), "⚠️  compliance unavailable: %v\n", err)
		return []complianceResult{}
	}
	return results
}

func loadEntryCompliance(entry lister.ListEntry) ([]complianceResult, error) {
	policies, err := loadPolicies()
	if err != nil {
		return nil, err
	}
	applicable := policies.Applicable(entry.Schema)
	stored := map[string]policyfacts.Stored{}
	if len(applicable) > 0 && entry.ResourceID != nil && *entry.ResourceID != "" {
		db, err := database.OpenCatalogDBReadOnly(effectivePudlDir())
		if err != nil {
			return nil, err
		}
		defer db.Close()
		rows, err := policyfacts.ReadStored(db, *entry.ResourceID, "")
		if err != nil {
			return nil, err
		}
		for _, s := range rows {
			stored[s.Policy] = s
		}
	}
	provisional := policies.Review(entry.Schema).Status != validator.ReviewValidated
	out := []complianceResult{}
	for _, p := range applicable {
		s, ok := stored[p.Name]
		if !ok {
			s = policyfacts.Stored{PolicyState: database.PolicyState{Policy: p.Name, Status: "not_evaluated"}, Violations: []policy.Violation{}}
		}
		out = append(out, complianceResult{Stored: s, Provisional: provisional, Stale: ok && policies.IsStale(s.PolicyState)})
	}
	return out, nil
}

func printCompliance(results []complianceResult) {
	if len(results) == 0 {
		return
	}
	fmt.Fprintln(outw(), "Compliance:")
	for _, r := range results {
		status := r.Status
		if r.Provisional {
			status += " (provisional: type is a draft)"
		}
		if r.HighestSeverity != "" {
			status += ", highest severity " + string(r.HighestSeverity)
		}
		if r.Stale {
			status += " [stale: run pudl policy evaluate]"
		}
		fmt.Fprintf(outw(), "  %s: %s\n", r.Policy, status)
		for _, v := range r.Violations {
			fmt.Fprintf(outw(), "    %-8s %s at %s (%s, declared by %s)\n", v.Severity, v.Rule, v.Path, v.Kind, v.DeclaredBy)
		}
	}
}
