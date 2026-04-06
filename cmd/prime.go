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

- **Catalog**: SQLite database at ~/.pudl/data/sqlite/catalog.db storing all
  imported entries with metadata, schema assignments, and content-addressed IDs.
- **Schemas**: CUE files in ~/.pudl/schema/ that define structure and validation
  rules. Organized by package (aws, k8s, etc.). Schema inference is automatic.
- **Definitions**: Named instances of schemas with concrete configuration and
  socket wiring to other definitions.
- **Fact store**: Bitemporal store for structured assertions (observations,
  dependencies, derived facts) with valid-time and transaction-time tracking.
- **Datalog rules**: CUE-defined rules evaluated over the fact store and catalog
  for derived queries.
- **Workspace**: A repo can have a .pudl/ directory for project-local config
  and rules. Global config lives at ~/.pudl/.

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
pudl export <id>                             # export raw data
pudl delete <id>                             # remove an entry
` + "```" + `

### Schema management
` + "```" + `
pudl schema list                             # list schemas by package
pudl schema show <name>                      # display schema CUE
pudl schema new <name>                       # generate from imported data
pudl schema add <name> <file>                # add schema file
pudl schema reinfer                          # re-run inference on entries
` + "```" + `

### Definitions
` + "```" + `
pudl definition list                         # list definitions
pudl definition show <name>                  # show definition details
pudl definition validate                     # validate all definitions
pudl definition graph                        # show dependency graph
` + "```" + `

### Recording observations
` + "```" + `
pudl observe "<description>" --kind <kind> --scope <repo:path>
` + "```" + `
Kinds: fact, obstacle, pattern, antipattern, suggestion, bug, opportunity
Scope format: repo:path (e.g. pudl:internal/database, myapp:pkg/auth)

As an agent, use this to record structured observations about the codebase:
` + "```" + `
pudl observe "auth module has no rate limiting" --kind suggestion --scope myapp:pkg/auth --source claude-code
pudl observe "circular dependency between user and auth" --kind obstacle --scope myapp:pkg --source claude-code
` + "```" + `

### Querying facts
` + "```" + `
pudl facts list --relation observation       # list observations
pudl facts list --relation observation --source claude-code
pudl facts show <id>                         # full fact details
pudl facts retract <id>                      # mark as wrong
pudl facts invalidate <id>                   # mark as no longer true
` + "```" + `

### Datalog queries
` + "```" + `
pudl query <relation>                        # query derived facts
pudl query <relation> --field=value          # filter results
pudl rule add <file.cue>                     # install a rule file
pudl rule add <file.cue> --global            # install globally
` + "```" + `
Rules are CUE files in .pudl/schema/pudl/rules/ (repo) or ~/.pudl/schema/pudl/rules/ (global).

### Workspace setup
` + "```" + `
pudl init                                    # initialize ~/.pudl/
pudl repo init                               # initialize .pudl/ in current repo
pudl doctor                                  # health check
pudl status                                  # workspace status
pudl repo validate                           # validate all schemas + definitions
` + "```" + `

## Conventions for agents

1. **Always pass --source** when using ` + "`pudl observe`" + ` so observations are
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
