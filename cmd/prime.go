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

const primeText = `# pudl — Personal Unified Data Lake

pudl is a CLI tool for managing a local data lake backed by SQLite. It imports
structured data (JSON, YAML, CSV, NDJSON), assigns CUE-based schemas, and
provides a bitemporal fact store with Datalog query support.

## Core concepts

- **Catalog**: SQLite at .pudl/data/sqlite/catalog.db inside a repository
  workspace, or ~/.pudl/data/sqlite/catalog.db in global mode. Repositories do
  not share mutable catalog state.
- **Schemas**: CUE files in the active .pudl/schema/ define structure and
  validation rules. Repository schemas are searched before global fallbacks.
- **Models**: ` + "`#SystemModel`" + ` definitions packaging a system's shape, how to
  populate (observe) it, optional desired state, and how to converge. Run with
  ` + "`pudl run`" + `.
- **Fact store**: Bitemporal store for structured assertions (observations,
  dependencies, derived facts) with valid-time and transaction-time tracking.
- **Datalog rules**: CUE-defined rules evaluated over the fact store and catalog
  for derived queries.
- **Workspace**: pudl repo init creates a self-contained .pudl/ with config,
  built-ins, models, raw data, metadata, and catalog. Global mode uses ~/.pudl/.

## Commands you should know

### Importing data
` + "```" + `
pudl import --path <file>                    # auto-detect format
pudl import --path <file> --schema <name>    # explicit schema
pudl import --path "*.json"                  # wildcard batch import
` + "```" + `

### Browsing the catalog
` + "```" + `
pudl list                                    # list all entries
pudl list --schema <name>                    # filter by schema
pudl show <id>                               # show entry details + content
pudl export --id <id>                        # export raw data
pudl delete <id>                             # remove an entry
` + "```" + `

### Schema management
` + "```" + `
pudl schema list                             # list schemas by package
pudl schema show <name>                      # display schema CUE
pudl schema new --from <id> --path <package>/#<Definition>
pudl schema add <name> <file>                # add schema file
pudl schema reinfer                          # re-run inference on entries
` + "```" + `

### Models (#SystemModel)
` + "```" + `
pudl model list                              # list registered models (+ last-run status)
pudl model show <name>                       # show populate/converge/desired/checks
pudl model validate <name>                   # structural validation without running
pudl run <name>                              # observe-only run (populate → drift → checks)
pudl run <name> --converge                   # close drift via mu
pudl run-set <models...>                     # observe exact producer/consumer set
pudl run-set <models...> --converge          # whole-set preflight, then mutate
pudl run-set report [run-set-id]             # durable orchestration report
pudl run-set resume|reject <run-set-id>      # decide a pending exact plan
pudl run <name> --check-upstream             # warn if a depends_on upstream is drifted/failed
pudl model deps                              # show the cross-model dependency graph (no run)
pudl model deps --derive                     # also derive edges from desired↔produced identities
` + "```" + `

### Cross-model dependencies

A ` + "`#SystemModel`" + ` can declare ` + "`depends_on: [\"<model>\", ...]`" + ` — the models whose
output it needs. ` + "`pudl run`" + ` / ` + "`pudl model deps`" + ` record these as
` + "`model_depends_on(from,to)`" + ` facts; built-in recursive rules reason over them:
` + "```" + `
pudl query depends_transitive from=<m>       # what <m> depends on (transitively)
pudl query impacted_by changed=<m>           # blast radius: who depends on <m>
pudl query cyclic                            # models in a dependency cycle
pudl query --topo model_depends_on           # topological run order (deps first)
` + "```" + `
` + "`pudl model deps`" + ` records edges for every registered model without running them;
` + "`--derive`" + ` adds heuristic edges from desired↔produced identity matching. pudl
makes deps queryable but does not re-run downstream models (that is mu's job).

### Cross-model value wiring

Model templates can declare required scalar ` + "`inputs`" + ` and ` + "`bindings`" + `. Both
the consumer slot and source schema field must opt into
` + "`@pudl(binding=plain)`" + `. A standalone run reuses an eligible successful
producer snapshot but never starts the producer. Use ` + "`pudl run-set <models...>`" + `
to name the closed set, order producers first, and pin current-run observations.

Sealed values stay in mu's provider channel and PUDL persists only schemes and
fingerprints. Generated targets require strict per-action claims. A run-set that
can write a sealed output always pauses for exact-plan approval, and resume
revalidates the plan. Every apply is guarded by mu's same-workspace raw plan
digest before provider access and producer-first execution.

### Writing data — three doors (do not confuse them)

There is exactly one door for each kind of write:

- **Assert a fact** (observation, feedback, any relation) → ` + "`pudl facts add`" + `
  (or the sugar ` + "`pudl facts observe`" + ` for observations).
- **Import data** into the lake (JSON/YAML/CSV files) → ` + "`pudl import`" + `.
- **Bridge to mu** (ingest mu observe/build results into the catalog) → ` + "`pudl mu …`" + `.

### Recording observations and facts
` + "```" + `
pudl facts observe "<description>" --kind <kind> --scope <repo:path>   # sugar
pudl facts add --relation <rel> --args '<json-object>'                 # canonical
` + "```" + `
Kinds: fact, obstacle, pattern, antipattern, suggestion, bug, opportunity
Scope format: repo:path (e.g. pudl:internal/database, myapp:pkg/auth)

` + "`facts add`" + ` is the one low-level write for any relation. Known agent
relations (observation, feedback) are validated against their built-in schema on
write (bad --kind or verdict is rejected). Examples:
` + "```" + `
pudl facts observe "auth has no rate limiting" --kind suggestion --scope myapp:pkg/auth --source claude-code
pudl facts add --relation feedback \
    --args '{"target":"<fact-id>","verdict":"helpful","source":"claude-code"}'
` + "```" + `
Feedback verdict is helpful | harmful | neutral; target is the fact/rule it concerns.

### Self-improvement loop

pudl runs a memory loop: observations you record mature from feedback, and the most
useful promoted ones are recalled into future sessions.
` + "```" + `
pudl memory context [--task "<text>"]        # ranked promoted knowledge (recall)
pudl facts curate                            # advance maturity from feedback (deterministic)
pudl memory cycle                            # run the full mu cycle (reflect → curate)
pudl hooks install                           # wire the loop into Claude Code (SessionStart/Stop)
` + "```" + `
Record feedback on facts/rules you act on so the curator can promote what works and
reject what doesn't.

### Querying and curating facts
` + "```" + `
pudl facts list --relation observation       # list observations
pudl facts list --relation observation --source claude-code
pudl facts search "<text>"                   # full-text search (FTS5; best matches first)
pudl facts show <id>                         # full fact details
pudl facts promote <id> --to reviewed        # advance maturity (raw→reviewed→promoted|rejected)
pudl facts promote <id> --to promoted --rule <ref>
pudl facts curate                            # auto-advance maturity from feedback (no LLM)
pudl facts retract <id>                      # mark as wrong (we erred)
pudl facts invalidate <id>                   # mark as no longer true (reality changed)
` + "```" + `

### Datalog queries
` + "```" + `
pudl query <relation>                        # query derived facts
pudl query <relation> key=value              # filter results (positional key=value, not --where)
pudl query --list                            # list queryable relations + their arg keys
pudl rule add <file.cue>                     # install a rule file
pudl rule add <file.cue> --global            # install globally
` + "```" + `
Rules are CUE files in .pudl/schema/pudl/rules/ (repo) or ~/.pudl/schema/pudl/rules/ (global).

### Workspace setup
` + "```" + `
pudl init                                    # initialize ~/.pudl/
pudl repo init                               # initialize .pudl/ in current repo
pudl doctor                                  # health check
pudl status                                  # recorded convergence status
` + "```" + `

## Conventions for agents

1. **Always pass --source** when using ` + "`pudl facts observe/add`" + ` so facts are
   attributable. Use your agent name (e.g. "claude-code").

2. **Use --scope with repo:path format** for observations so they are globally
   unambiguous and joinable across repositories.

3. **Use --json** flag on any command when you need machine-readable output
   for further processing.

4. **IDs are content-addressed** (SHA256). You can use short prefixes when
   they are unambiguous.

5. **Temporal queries**: use --as-of-valid and --as-of-tx flags on fact
   queries to ask "what was true at time X" or "what did we believe at time X".

6. **Schema inference is automatic** on import. You usually don't need to
   specify --schema unless you want to force a specific one.
`
