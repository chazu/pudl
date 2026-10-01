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
		fmt.Print(primeText)
	},
}

func init() {
	rootCmd.AddCommand(primeCmd)
}

const primeText = `# PUDL for humans and agents

PUDL retains typed observations of systems, compares them with declared
expectations, and records evidence. Mu performs external actions and handles
secrets. Ordinary runs observe; --converge explicitly enables mutation.

## Start with the existing workspace

pudl model list
pudl model show <name> --json
pudl run <name>
pudl run report --json

Create local state with pudl init; use pudl init --global for ~/.pudl.
Repository catalogs and mutable state remain isolated beneath .pudl/.
Use pudl help <command> --json or pudl guide <topic> for targeted instructions.

## Exact sets and convergence

pudl run set <producer> <consumer>
pudl run set <models...> --converge --require-approval
pudl run resume <operation-id>
pudl run reject <operation-id>

An exact set runs only the named models in dependency order. Missing binding
producers are errors. Standalone runs can reuse eligible recorded producer
snapshots; they never start a producer automatically. Plain bindings pin scoped
snapshots. Sealed values stay inside Mu, and sets that can write sealed outputs
require exact-plan approval. Resume rejects a changed set plan.

Report completion, resource conformity, checks, and evidence verification are
separate dimensions. Successful execution can still find drift. Stored replay
does not establish fresh live verification. An uncertain mutation needs
verification; a scoped clean result does not prove the whole model is clean.

## Imported evidence and schemas

pudl import --path <file>
pudl list --json
pudl show <id> --raw
pudl schema list --json
pudl schema show <package.#Definition>
pudl schema new --from <id> --path <package>/#<Definition>
pudl doctor --json

Formats and schemas are inferred on import. --origin is an explicit list filter.
Saved Mu observations are ingested with pudl mu ingest-observe; replay requires
run --from-catalog --catalog-scope <snapshot-or-origin> and remains unverified.
Schema/Git and module/CUE helpers are available through their command groups.

## Generic facts and rules

pudl facts add --relation <name> --args '<json-object>' --source <origin>
pudl facts list --relation <name> --json
pudl facts search "<text>" --json
pudl facts show <id> --json
pudl facts retract <id>
pudl facts invalidate <id>
pudl query <relation> key=value --json
pudl rule add <file.cue>

Facts are arbitrary JSON assertions with valid-time and transaction-time
history. --schema on facts add explicitly validates against an authored CUE
schema. Retraction withdraws an assertion; invalidation records that reality
changed. --as-of-valid and --as-of-tx select historical facts. Datalog supports
joins and recursion, including model dependency and impact queries.

Use --json for supported structured views. Attribute fact writes with --source.
Use retained IDs to inspect evidence; content hashes and provenance remain
available after subsequent observations.
`
