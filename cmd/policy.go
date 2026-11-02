package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/policyfacts"
)

// policyCmd groups the compliance policy commands. Policies are schemas with
// `_pudl.schema_type: "policy"`, evaluated per resource; results are the
// policy_status and policy_violation facts.
var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Evaluate compliance policies against cataloged resources",
	Long: `Compliance policies are schemas with _pudl.schema_type: "policy" that name
their type through _pudl.base_schema. Every import and observation evaluates the
applicable policies per resource and stores the results as facts:

  policy_status(resource, policy, type, status, policy_digest)
  policy_violation(resource, policy, rule, severity, path, kind, declared_by)

Status is compliant, noncompliant or unknown (a constrained field was absent).
The built-in rule policy_noncompliant(resource, policy, severity) serves checks.
A result is stale when its digest differs from the loaded policy's; pudl doctor
reports stale results and pudl policy evaluate refreshes them.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// loadPolicies loads the compliance policies of the active schema paths.
func loadPolicies() (*policyfacts.Evaluator, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inferrer, err := inference.Shared(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("init schema inferrer: %w", err)
	}
	return policyfacts.New(inferrer), nil
}

func init() {
	rootCmd.AddCommand(policyCmd)
}
