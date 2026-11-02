package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/policy"
	"github.com/chazu/pudl/internal/policyfacts"
)

var policyShowLimit int

var policyShowCmd = &cobra.Command{
	Use:   "show POLICY",
	Short: "Show a policy's rules, inheritance and failing resources",
	Long: `Show one compliance policy: its inheritance chain, its rules (inherited ones
included) with severity, doc and the policy that declared each, how many
resources currently fail each rule, its stored result counts and staleness, and
the resources failing most (most severe first, then most violations).

--limit caps the failing resources listed (0 lists all).`,
	Example: `  pudl policy show pudl/aws.#BucketBestPractice
  pudl policy show pudl/aws.#BucketBestPractice --limit 0 --json`,
	Args: cobra.ExactArgs(1),
	RunE: pudlRunE(runPolicyShow),
}

type policyShowOutput struct {
	ReportVersion int                 `json:"report_version"`
	Summary       policyfacts.Summary `json:"summary"`
	Chain         []string            `json:"chain"`
	Rules         []ruleRow           `json:"rules"`
	// Failing are the resources failing most, capped by --limit;
	// FailingTotal counts them all.
	Failing      []policyfacts.Stored `json:"failing"`
	FailingTotal int                  `json:"failing_total"`
}

// ruleRow is a rule and the number of resources currently failing it.
type ruleRow struct {
	policy.Rule
	Failing int `json:"failing"`
}

func runPolicyShow(cmd *cobra.Command, args []string) error {
	policies, db, err := openPolicies()
	if err != nil {
		return err
	}
	defer db.Close()
	selected, err := policies.Select(args)
	if err != nil {
		return err
	}
	p := selected[0]
	stored, err := policyfacts.ReadStored(db, "", p.Name)
	if err != nil {
		return err
	}
	states := make([]database.PolicyState, len(stored))
	for i, s := range stored {
		states[i] = s.PolicyState
	}
	out := policyShowOutput{
		ReportVersion: policyReportVersion,
		Summary:       policies.Summarize(states, selected)[0],
		Chain:         p.Chain,
		Rules:         ruleRows(p, stored),
		Failing:       failingMost(stored),
	}
	out.FailingTotal = len(out.Failing)
	if policyShowLimit > 0 && len(out.Failing) > policyShowLimit {
		out.Failing = out.Failing[:policyShowLimit]
	}
	if jsonOutput {
		return printJSON(out)
	}
	printPolicyShow(out)
	return nil
}

// ruleRows counts, per rule, the resources failing it. A violation of an
// unannotated constraint is named by its path and gets its own row.
func ruleRows(p *policy.Policy, stored []policyfacts.Stored) []ruleRow {
	rows := make([]ruleRow, len(p.Rules))
	index := map[string]int{}
	for i, r := range p.Rules {
		rows[i], index[r.Name] = ruleRow{Rule: r}, i
	}
	for _, s := range stored {
		seen := map[string]bool{}
		for _, v := range s.Violations {
			i, ok := index[v.Rule]
			if !ok {
				i, index[v.Rule] = len(rows), len(rows)
				rows = append(rows, ruleRow{Rule: policy.Rule{Name: v.Rule, Severity: v.Severity, Paths: []string{v.Path}, DeclaredBy: v.DeclaredBy}})
			}
			if !seen[v.Rule] {
				seen[v.Rule] = true
				rows[i].Failing++
			}
		}
	}
	return rows
}

// failingMost orders the noncompliant results most severe first, then by
// violation count.
func failingMost(stored []policyfacts.Stored) []policyfacts.Stored {
	failing := []policyfacts.Stored{}
	for _, s := range stored {
		if len(s.Violations) > 0 {
			failing = append(failing, s)
		}
	}
	sort.SliceStable(failing, func(i, j int) bool {
		a, b := failing[i], failing[j]
		if ra, rb := a.HighestSeverity.Rank(), b.HighestSeverity.Rank(); ra != rb {
			return ra > rb
		}
		return len(a.Violations) > len(b.Violations)
	})
	return failing
}

func printPolicyShow(out policyShowOutput) {
	s := out.Summary
	fmt.Fprintf(outw(), "Policy: %s\n", s.Policy)
	fmt.Fprintf(outw(), "Type: %s (%s)\n", s.Type, s.Review)
	fmt.Fprintf(outw(), "Extends: %s\n", orDash(s.Extends))
	fmt.Fprintf(outw(), "Chain: %s\n", strings.Join(out.Chain, " -> "))
	fmt.Fprintf(outw(), "Last evaluated: %s\n", evaluatedAt(s.LastEvaluated))
	fmt.Fprintf(outw(), "Results: %d compliant, %d noncompliant, %d unknown, %d stale\n", s.Compliant, s.Noncompliant, s.Unknown, s.Stale)
	if s.Stale > 0 {
		fmt.Fprintf(outw(), "⚠️  %d results are stale: run pudl policy evaluate\n", s.Stale)
	}
	fmt.Fprintf(outw(), "Rules (%d):\n", len(out.Rules))
	for _, r := range out.Rules {
		fmt.Fprintf(outw(), "  %-8s %s (declared by %s): %d failing\n", r.Severity, r.Name, r.DeclaredBy, r.Failing)
		if r.Doc != "" {
			fmt.Fprintf(outw(), "           %s\n", r.Doc)
		}
	}
	if out.FailingTotal == 0 {
		fmt.Fprintln(outw(), "No failing resources.")
		return
	}
	fmt.Fprintf(outw(), "Failing resources (%d of %d):\n", len(out.Failing), out.FailingTotal)
	for _, f := range out.Failing {
		var rules []string
		for _, v := range f.Violations {
			rules = append(rules, v.Rule)
		}
		fmt.Fprintf(outw(), "  %-8s %s (entry %s): %s\n", f.HighestSeverity, f.ResourceID, f.EntryID, strings.Join(rules, ", "))
	}
}

func init() {
	policyCmd.AddCommand(policyShowCmd)
	policyShowCmd.Flags().IntVar(&policyShowLimit, "limit", 10, "Maximum failing resources to list (0 = all)")
}
