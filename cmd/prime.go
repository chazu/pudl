package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Output agent prompt describing how to use pudl",
	Long: `Print a structured prompt that teaches AI agents how to use pudl.

Include a line like this in your CLAUDE.md or similar agent config:

    Run 'pudl prime' to learn how to use the pudl data lake CLI.

The agent will then know to execute the command and read the output
to understand pudl's capabilities and conventions.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(outw(), primeText)
	},
}

func init() {
	rootCmd.AddCommand(primeCmd)
}

const primeText = `# PUDL for humans and agents

PUDL retains typed observations of systems, compares them with declared
expectations, and records evidence. It never changes the systems it observes:
a check reports drift, and the tools that own those systems apply changes.

## Start with the existing workspace

pudl model list
pudl model show <name> --json
pudl check <name>
pudl report --json

Create local state with pudl init; use pudl init --global for ~/.pudl.
Repository catalogs and mutable state remain isolated beneath .pudl/.
Use pudl help <command> --json or pudl guide <topic> for targeted instructions.

## Observe with commands

pudl model new <name> --populate 'command:<cmdline printing JSON>'
pudl check --populate 'command:gcloud compute networks list --format=json'
pudl check <name> --detailed-exitcode

A model's populate arm (#CommandObserve) runs commands that print JSON records,
such as gcloud, kubectl or aws with JSON output, and ingests them as one
snapshot. A model with desired state reports drift against that snapshot.

## Several models

pudl check <producer> <consumer>

A check runs only the named models, producers first by depends_on, and records
one report for all of them. It never starts a model that was not named.

Report completion, resource conformity, checks, and evidence verification are
separate dimensions. Successful execution can still find drift. Stored replay
does not establish fresh live verification.

## Investigate and retain evidence

pudl example install gcp-network-hygiene
pudl list --where name=example --select name --all --json
pudl show <id> --field metadata.name
pudl show <id> --history --json
pudl snapshot show <new> --compare <old> --json
pudl model new <name> --check <relation> --evidence scope:<population> --max-age 15m

Checks carry pass/fail/unknown/error outcomes and structured diagnostics.
Missing or incomplete evidence is not a pass. Use explicit snapshot evidence
and completeness declarations for inventory checks. Summaries separate execution,
conformity, checks and verification. age_at_check is historical, not a fresh age.
The GCP example defaults to local fixture data; its live model is separately named.

Project definitions are local or explicitly vendored through workspace.cue.
pudl config --paths --json shows effective paths; --global selects personal state.
Doctor's repairs list gives reviewable preview/apply arguments; it applies nothing.

## Imported evidence and schemas

pudl import --path <file>
pudl list --json
pudl show <id> --raw
pudl schema list --json
pudl schema show <package.#Definition>
pudl schema new --from <id> --path <package>/#<Definition>
pudl doctor --json

Formats and schemas are inferred on import. --origin is an explicit list filter.
Replaying a stored snapshot requires check <model> --from-catalog
--catalog-scope <snapshot-id> and remains unverified.
Use Git, your editor, and cue on the paths from pudl config --paths --json.

## Facts and rules

pudl query <relation> key=value --json
pudl facts list --relation <name> --json
pudl facts search "<text>" --json
pudl facts show <id> --json
pudl rule add <file.cue>

Read the fact store through pudl query; facts list/search/show inspect stored
facts directly. Facts are arbitrary JSON assertions with valid-time and
transaction-time history; --as-of-valid and --as-of-tx select historical facts.
Datalog supports joins and recursion, including model dependency and impact
queries.

Fact writes are advanced commands, hidden from everyday help: facts add
--relation <name> --args '<json-object>' --source <origin> records an assertion
(--schema validates it against an authored CUE schema), facts retract withdraws
one, and facts invalidate records that reality changed. See pudl guide facts.

Use --json for supported structured views. Attribute fact writes with --source.
Use retained IDs to inspect evidence; content hashes and provenance remain
available after subsequent observations.
`
