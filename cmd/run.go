package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// Flag variables for `pudl run`. They are read exactly once, by
// runOptionsFromFlags; nothing else reads or writes them.
var (
	runMuRoot            string
	runConverge          bool
	runOnly              []string
	runDryRun            bool
	runMaxIters          int
	runMaxApplies        int
	runFromCatalog       bool
	runCatalogScope      string
	runCheckUpstream     bool
	runPopulateSpec      string
	runPopulateInput     []string
	runRequireApproval   bool
	runMaxObservationAge time.Duration
)

// Defaults for the convergence caps, shared by the flags and by run-set members
// (which run with default settings).
const (
	defaultRunMaxIters   = 5
	defaultRunMaxApplies = 20
)

var runCmd = &cobra.Command{
	Use:   "run [<model>]",
	Short: "Run a #SystemModel instance (observe-only, or --converge)",
	Long: `Run a #SystemModel instance through the ACUTE cycle.

<model> is a registered #SystemModel — a definition inheriting #SystemModel,
resolved by name (its name field or short definition name) from the project
.pudl/schema first, then the global ~/.pudl/schema. Register one with
"pudl schema add". Default is OBSERVE-ONLY: populate -> drift -> checks ->
report, no mutation. Pass --converge to close drift; see the V1 build spec.

Examples:
    pudl run github-chazu
    pudl run k8sPolicy --converge
    pudl run k8sConverge --converge --only web,api
    pudl run k8sConverge --converge --dry-run`,
	Args: func(cmd *cobra.Command, args []string) error {
		if runPopulateSpec != "" {
			if len(args) != 0 {
				return fmt.Errorf("--populate is an ad-hoc run and does not take a model name")
			}
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := executeRun(runOptionsFromFlags(cmd, args), defaultRunDeps())
		return err
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVar(&runPopulateSpec, "populate", "", "Run an unregistered observer as plugin:<name>")
	runCmd.Flags().StringArrayVar(&runPopulateInput, "input", nil, "Ad-hoc populate input key=value (repeatable)")
	runCmd.Flags().StringVar(&runMuRoot, "mu-root", "", "mu project root to run within (default: discover mu.cue from the model dir)")
	runCmd.Flags().BoolVar(&runConverge, "converge", false, "opt into the convergence loop (mutates the target)")
	runCmd.Flags().StringSliceVar(&runOnly, "only", nil, "converge only these resource selectors (requires --converge)")
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "print the plan, execute nothing (requires --converge)")
	runCmd.Flags().IntVar(&runMaxIters, "max-iters", defaultRunMaxIters, "loop iteration cap (requires --converge)")
	runCmd.Flags().IntVar(&runMaxApplies, "max-applies", defaultRunMaxApplies, "durable cap on applies since this model was last verified clean; 0 disables (requires --converge)")
	runCmd.Flags().BoolVar(&runFromCatalog, "from-catalog", false, "drift over already-ingested records (inventory; no live observe); requires --catalog-scope")
	runCmd.Flags().StringVar(&runCatalogScope, "catalog-scope", "", "which already-ingested records --from-catalog replays: an observe snapshot ID, or the origin they were ingested under")
	runCmd.Flags().BoolVar(&runCheckUpstream, "check-upstream", false, "warn if any transitive upstream model (depends_on) is drifted/failed")
	runCmd.Flags().DurationVar(&runMaxObservationAge, "max-observation-age", 0, "reject a bound producer snapshot older than this duration")
	runCmd.Flags().BoolVar(&runRequireApproval, "require-approval", false, "persist the converge request and wait for `pudl run resume <run-id>`")
}
