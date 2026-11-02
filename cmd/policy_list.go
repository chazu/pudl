package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/policyfacts"
)

var policyListType string

var policyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List loaded policies with their stored result counts",
	Long: `List every loaded compliance policy: the type it applies to, the policy it
extends, its rule count, the type's review status (draft results are
provisional), when it was last evaluated, and its stored compliant,
noncompliant, unknown and stale result counts. A result is stale when it was
evaluated with an older version of the policy; pudl policy evaluate refreshes it.

--type T lists the policies that apply to type T.`,
	Example: `  pudl policy list
  pudl policy list --type pudl/aws.#Bucket --json`,
	Args: cobra.NoArgs,
	RunE: pudlRunE(runPolicyList),
}

// policyReportVersion versions the --json output of policy list and show.
const policyReportVersion = 1

type policyListOutput struct {
	ReportVersion int                   `json:"report_version"`
	Policies      []policyfacts.Summary `json:"policies"`
	LoadErrors    []string              `json:"load_errors,omitempty"`
}

func runPolicyList(cmd *cobra.Command, args []string) error {
	policies, db, err := openPolicies()
	if err != nil {
		return err
	}
	defer db.Close()
	selected, _ := policies.Select(nil)
	if policyListType != "" {
		selected = policies.Applicable(policyListType)
	}
	states, err := db.ListPolicyState("", "")
	if err != nil {
		return err
	}
	out := policyListOutput{ReportVersion: policyReportVersion, Policies: policies.Summarize(states, selected)}
	for _, le := range policies.LoadErrors() {
		out.LoadErrors = append(out.LoadErrors, le.Error())
	}
	if jsonOutput {
		return printJSON(out)
	}
	if len(out.Policies) == 0 {
		fmt.Fprintln(outw(), "No compliance policies loaded.")
	} else {
		w := tabwriter.NewWriter(outw(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "POLICY\tTYPE\tEXTENDS\tRULES\tREVIEW\tLAST EVALUATED\tCOMPLIANT\tNONCOMPLIANT\tUNKNOWN\tSTALE")
		for _, r := range out.Policies {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%d\t%d\t%d\t%d\n", r.Policy, r.Type, orDash(r.Extends), r.Rules,
				r.Review, evaluatedAt(r.LastEvaluated), r.Compliant, r.Noncompliant, r.Unknown, r.Stale)
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	for _, e := range out.LoadErrors {
		fmt.Fprintf(errw(), "⚠️  %s\n", e)
	}
	return nil
}

// openPolicies loads the policies and opens the catalog holding their results.
func openPolicies() (*policyfacts.Evaluator, *database.CatalogDB, error) {
	policies, err := loadPolicies()
	if err != nil {
		return nil, nil, err
	}
	db, err := database.NewCatalogDB(effectivePudlDir())
	return policies, db, err
}

func evaluatedAt(unix int64) string {
	if unix == 0 {
		return "never"
	}
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

func init() {
	policyCmd.AddCommand(policyListCmd)
	policyListCmd.Flags().StringVar(&policyListType, "type", "", "List only the policies that apply to type `T`")
}
