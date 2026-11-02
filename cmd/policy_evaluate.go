package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/policyfacts"
)

var (
	policyEvaluateDryRun bool
	policyEvaluateWhatIf string
)

var policyEvaluateCmd = &cobra.Command{
	Use:   "evaluate [POLICY...]",
	Short: "Re-evaluate policies against stored current resources",
	Long: `Re-evaluate compliance policies against every stored current resource, from
its retained payload, without re-importing. Changed results replace the current
policy_status and policy_violation facts; history keeps the previous result and
the policy digest it was evaluated with. Results of policies that no longer apply,
or are no longer loaded, are closed. A policy that fails to load is reported and
its results are left unchanged.

Name policies to evaluate only those. --dry-run reports what would change and
writes nothing.

--what-if FILE evaluates a draft policy file that is not installed: it is loaded
beside the installed schemas (shadowing an installed policy of the same name),
evaluated against the stored current resources, and reported as status counts and
the top violations. Nothing is written. The draft must be standalone CUE (its
policies name their type through _pudl.base_schema).`,
	Example: `  pudl policy evaluate
  pudl policy evaluate pudl/aws.#S3BucketBestPractice --dry-run
  pudl policy evaluate --what-if drafts/s3_strict.cue --json`,
	RunE: pudlRunE(runPolicyEvaluate),
}

// policyEvaluateReportVersion versions the --json output.
const policyEvaluateReportVersion = 1

type policyEvaluateOutput struct {
	ReportVersion int    `json:"report_version"`
	Mode          string `json:"mode"` // evaluate | dry-run | what-if
	Draft         string `json:"draft,omitempty"`
	policyfacts.Report
}

func runPolicyEvaluate(cmd *cobra.Command, args []string) error {
	if policyEvaluateWhatIf != "" && policyEvaluateDryRun {
		return fmt.Errorf("--what-if never writes; drop --dry-run")
	}
	db, err := database.NewCatalogDB(effectivePudlDir())
	if err != nil {
		return err
	}
	defer db.Close()
	out := policyEvaluateOutput{ReportVersion: policyEvaluateReportVersion, Mode: "evaluate"}
	if policyEvaluateWhatIf != "" {
		out.Mode, out.Draft = "what-if", policyEvaluateWhatIf
		out.Report, err = whatIfPolicies(cmd, db, args)
	} else {
		if policyEvaluateDryRun {
			out.Mode = "dry-run"
		}
		var policies *policyfacts.Evaluator
		if policies, err = loadPolicies(); err == nil {
			out.Report, err = policies.Refresh(cmd.Context(), db, loadEntryPayload, policyfacts.Options{Policies: args, DryRun: policyEvaluateDryRun})
		}
	}
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(out)
	}
	printPolicyEvaluation(out)
	return nil
}

// whatIfPolicies evaluates the draft file's policies (or those of them named
// in args) without writing.
func whatIfPolicies(cmd *cobra.Command, db *database.CatalogDB, args []string) (policyfacts.Report, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return policyfacts.Report{}, err
	}
	draft, err := policyfacts.LoadDraft(policyEvaluateWhatIf, effectiveSchemaPaths(cfg))
	if err != nil {
		return policyfacts.Report{}, err
	}
	defer draft.Close()
	selected := draft.Policies
	if len(args) > 0 {
		if selected, err = draft.Select(args); err != nil {
			return policyfacts.Report{}, err
		}
	}
	report, err := draft.Evaluator.Preview(cmd.Context(), db, loadEntryPayload, selected)
	report.LoadErrors = nil // installed policies' load errors are not the draft's
	return report, err
}

func printPolicyEvaluation(out policyEvaluateOutput) {
	switch out.Mode {
	case "what-if":
		fmt.Fprintf(outw(), "What-if evaluation of %s (nothing written)\n", out.Draft)
	case "dry-run":
		fmt.Fprintln(outw(), "Policy evaluation dry run (nothing written)")
	default:
		fmt.Fprintln(outw(), "Policy evaluation")
	}
	fmt.Fprintf(outw(), "  %d resources evaluated\n", out.Resources)
	for _, p := range out.Policies {
		fmt.Fprintf(outw(), "  %s (%s): %d compliant, %d noncompliant, %d unknown", p.Policy, p.Type, p.Compliant, p.Noncompliant, p.Unknown)
		if p.Provisional > 0 {
			fmt.Fprintf(outw(), " [%d provisional: type not validated]", p.Provisional)
		}
		fmt.Fprintln(outw())
	}
	if len(out.TopViolations) > 0 {
		fmt.Fprintln(outw(), "Top violations:")
		for _, v := range out.TopViolations {
			fmt.Fprintf(outw(), "  %-8s %s (%s, %s): %d resources\n", v.Severity, v.Rule, v.Policy, v.Kind, v.Resources)
		}
	}
	if out.Mode != "what-if" {
		verb := "updated"
		if out.Mode == "dry-run" {
			verb = "would update"
		}
		fmt.Fprintf(outw(), "Results: %d %s, %d closed, %d orphaned closed\n", out.Updated, verb, out.Closed, out.Orphaned)
	}
	for _, e := range out.LoadErrors {
		fmt.Fprintf(errw(), "⚠️  %s (existing results unchanged)\n", e)
	}
	for _, e := range out.Errors {
		fmt.Fprintf(errw(), "⚠️  evaluation failed for %s\n", e)
	}
}

func init() {
	policyCmd.AddCommand(policyEvaluateCmd)
	policyEvaluateCmd.Flags().BoolVar(&policyEvaluateDryRun, "dry-run", false, "Report what would change without writing")
	policyEvaluateCmd.Flags().StringVar(&policyEvaluateWhatIf, "what-if", "", "Evaluate a draft policy `FILE` without installing it or writing results")
}
