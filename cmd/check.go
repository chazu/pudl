package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Flag variables for `pudl check`. They are read exactly once, by
// checkOptionsFromFlags; nothing else reads or writes them.
var (
	checkFromCatalog      bool
	checkCatalogScope     string
	checkCheckUpstream    bool
	checkPopulateSpec     string
	checkDetailedExitCode bool
)

var checkCmd = &cobra.Command{
	Use:   "check <model>...",
	Short: "Observe and evaluate exactly the named models; one report",
	Long: `Observe and evaluate exactly the named #SystemModel instances.

Each <model> is a registered #SystemModel, resolved by name (its name field or
short definition name) from the project .pudl/schema and explicitly vendored
packages. Register one with "pudl schema add".

A check runs only the models it names, producers first: a model runs after
every named model it lists in depends_on. A model whose named producer does
not succeed is recorded as blocked. Models that were not named never run.

Each model is observe-only: populate -> drift -> checks. A model without a
populate arm is checks-only: it evaluates its checks over what the catalog
already holds (e.g. data brought in with pudl import). A #CommandObserve
populate runs plain commands that print JSON records (gcloud, kubectl,
aws ... --format=json). Nothing is changed.

A check records one report, whether it names one model or several. Read it
again with "pudl report [ID]".

With --detailed-exitcode the exit status reports the result: 0 clean,
2 drift or a failing fail-severity check, 1 error.

Examples:
    pudl check github-chazu
    pudl check network app --detailed-exitcode
    pudl check git-inventory --from-catalog --catalog-scope snap_...
    pudl check --populate 'command:gcloud compute networks list --format=json'`,
	Args: func(cmd *cobra.Command, args []string) error {
		if checkPopulateSpec != "" {
			if len(args) != 0 {
				return fmt.Errorf("--populate is an ad-hoc check and does not take a model name")
			}
			return nil
		}
		if checkFromCatalog && len(args) != 1 {
			return fmt.Errorf("--from-catalog replays one model's records; name exactly one model")
		}
		return cobra.MinimumNArgs(1)(cmd, args)
	},
	SilenceUsage: true,
	// The retired `pudl run` and `pudl run set` point here.
	SuggestFor: []string{"run"},
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := executeCheck(cmd.Context(), args, checkOptionsFromFlags())
		if !checkDetailedExitCode {
			return err
		}
		return silenceBareExit(cmd, detailedCheckExit(result, err))
	},
}

// checkOptions is the complete input of one `pudl check`, read once from its
// flags. Every member run receives the same options.
type checkOptions struct {
	runFlags
	populateSpec  string
	checkUpstream bool
}

func checkOptionsFromFlags() checkOptions {
	return checkOptions{
		runFlags:      runFlags{fromCatalog: checkFromCatalog, catalogScope: checkCatalogScope},
		populateSpec:  checkPopulateSpec,
		checkUpstream: checkCheckUpstream,
	}
}

// memberRunOptions is the observe-only run one member of a check performs.
func (o checkOptions) memberRunOptions(model string) runOptions {
	return runOptions{runFlags: o.runFlags, model: model, populateSpec: o.populateSpec, checkUpstream: o.checkUpstream}
}

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.Flags().StringVar(&checkPopulateSpec, "populate", "", "Check an unregistered observer: command:<cmdline> (a command printing JSON records)")
	checkCmd.Flags().BoolVar(&checkFromCatalog, "from-catalog", false, "drift over already-ingested records (inventory; no live observe); requires --catalog-scope")
	checkCmd.Flags().StringVar(&checkCatalogScope, "catalog-scope", "", "which already-ingested records --from-catalog replays: an observe snapshot ID, or the origin they were ingested under")
	checkCmd.Flags().BoolVar(&checkCheckUpstream, "check-upstream", false, "warn if any transitive upstream model (depends_on) is drifted/failed")
	checkCmd.Flags().BoolVar(&checkDetailedExitCode, "detailed-exitcode", false, detailedExitCodeUsage)
	checkCmd.Flags().BoolVar(&runProgressJSON, "progress-json", false, "Emit phase progress events as JSON lines on stderr (alongside diagnostics)")
}
