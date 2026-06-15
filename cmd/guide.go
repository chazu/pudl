package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var guideCmd = &cobra.Command{
	Use:   "guide [topic]",
	Short: "Quick-reference guide for agents and humans",
	Long: `Print usage guides for pudl features and concepts.

Run 'pudl guide' with no arguments to see all available topics.
Run 'pudl guide <topic>' to read a specific guide.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			printGuideIndex()
			return
		}
		topic := args[0]
		fn, ok := guideTopics[topic]
		if !ok {
			fmt.Fprintf(os.Stderr, "pudl guide: unknown topic %q\n", topic)
			fmt.Fprintln(os.Stderr, "Run 'pudl guide' for a list of topics.")
			os.Exit(2)
		}
		fn()
	},
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		topics := make([]string, 0, len(guideTopics))
		for k := range guideTopics {
			topics = append(topics, k)
		}
		return topics, cobra.ShellCompDirectiveNoFileComp
	},
}

var guideTopics = map[string]func(){
	"overview":    printGuideOverview,
	"import":      printGuideImport,
	"schemas":     printGuideSchemas,
	"facts":       printGuideFacts,
	"datalog":     printGuideDatalog,
	"definitions": printGuideDefinitions,
	"drift":       printGuideDrift,
	"pith":        printGuidePith,
	"mu":          printGuideMu,
	"agents":      printGuideAgents,
}

func init() {
	rootCmd.AddCommand(guideCmd)
}

func printGuideIndex() {
	fmt.Print(`pudl guide — quick-reference for agents and humans

Start here if you're new to pudl:

  pudl guide overview       What pudl is, the mental model, and where to go next

Data management:

  pudl guide import         Importing data: formats, schemas, wildcards, stdin
  pudl guide schemas        CUE schema system: inference, authoring, versioning

Reasoning and state:

  pudl guide facts          Bitemporal fact store: observations, retraction, time-travel
  pudl guide datalog        Datalog query engine: rules, recursive evaluation
  pudl guide definitions    Named schema instances, sockets, dependency graphs

Convergence:

  pudl guide drift          Drift detection between declared and live state
  pudl guide mu             How pudl and mu work together (ACUTE loop)

Programmability:

  pudl guide pith           Pith VM: stack-based programs against the data lake

For agents:

  pudl guide agents         Conventions, best practices, and tips for AI agents
`)
}

func printGuideOverview() {
	fmt.Print(`pudl guide overview — what pudl is, in 60 seconds

WHAT PUDL IS

  pudl is a personal unified data lake. You import structured data
  (JSON, YAML, CSV, NDJSON) into a local SQLite catalog, pudl infers
  CUE schemas, and you query, validate, and reason over the data
  using a bitemporal fact store and Datalog engine.

  pudl is not opinionated about what data you store — cloud inventory,
  configuration files, API responses, build artifacts, and observations
  all live in the same catalog with the same schema system.

THE MENTAL MODEL

  - catalog.db       stores all imported entries with metadata
  - schemas          CUE files that define structure and validation
  - definitions      named instances of schemas with concrete config
  - fact store       bitemporal assertions (valid-time + transaction-time)
  - datalog rules    CUE-defined rules for derived queries
  - pith VM          stack-based programs for data lake operations
  - workspace        .pudl/ (repo-local) + ~/.pudl/ (global)

THE DAY-TO-DAY VERBS

  pudl import <file>          Import data (auto-detects format + schema).
  pudl list                   Browse catalog entries.
  pudl show <id>              Inspect an entry's content and metadata.
  pudl facts list             Query the fact store.
  pudl query <relation>       Run Datalog queries over derived facts.
  pudl facts observe "<text>"       Record a structured observation.
  pudl status                 Show convergence status of definitions.
  pudl doctor                 Health check the workspace.

WHERE DATA LIVES

  ~/.pudl/data/sqlite/catalog.db    The catalog database.
  ~/.pudl/schema/                   Global schema repository (git-tracked).
  .pudl/schema/                     Repo-local schemas and rules.
  .pudl/config.cue                  Repo-local configuration.

WHAT TO READ NEXT

  Getting started:     pudl guide import → pudl guide schemas
  Recording state:     pudl guide facts → pudl guide datalog
  Infrastructure:      pudl guide definitions → pudl guide drift
  Programmability:     pudl guide pith
  Integration with mu: pudl guide mu
  For AI agents:       pudl guide agents
`)
}

func printGuideImport() {
	fmt.Print(`pudl guide import — importing data into the catalog

USAGE

  pudl import --path <file> [flags]

FORMATS

  pudl auto-detects format from file extension:

    .json      JSON (single object or array)
    .yaml/.yml YAML documents
    .csv        CSV (first row = headers)
    .ndjson     Newline-delimited JSON (one record per line)

  Override with --format if needed.

BASIC EXAMPLES

  pudl import --path inventory.json
  pudl import --path config.yaml --schema myapp.#Config
  pudl import --path "data/*.json"               # wildcard batch
  pudl import --path data.json --tag env:prod     # add tags

STDIN SUPPORT

  Pipe data directly into pudl:

    curl -s https://api.example.com/data | pudl import --stdin
    cat <<'EOF' | pudl import --stdin --format json
    {"name": "test", "value": 42}
    EOF

SCHEMA INFERENCE

  On import, pudl automatically:

  1. Detects the data format
  2. Infers a CUE schema from the data structure
  3. Matches against existing schemas (exact or structural)
  4. Assigns the best-matching schema or creates a new one

  Use --schema to force a specific schema assignment.
  Use --skip-inference to import without schema assignment.

CONTENT-ADDRESSED IDS

  Every imported entry gets a SHA256 content-addressed ID displayed
  in proquint format (e.g. "babam-babam"). Re-importing identical
  data produces the same ID (idempotent).

ENVELOPES

  pudl auto-detects envelope JSON with shape:

    {"schema": {...}, "definitions": [...], "data": <payload>}

  This is how mu plugins emit typed output. The schema metadata
  routes classification automatically.

WILDCARDS

  Glob patterns expand against the filesystem:

    pudl import --path "logs/**/*.json"
    pudl import --path "*.yaml"

  Each matching file is imported as a separate entry.

FLAGS

  --path <path>       File path or glob pattern (required unless --stdin)
  --stdin             Read data from stdin
  --format <fmt>      Force format (json, yaml, csv, ndjson)
  --schema <name>     Force schema assignment
  --tag <key:value>   Add metadata tags (repeatable)
  --skip-inference    Skip schema inference
  --json              Output results as JSON
`)
}

func printGuideSchemas() {
	fmt.Print(`pudl guide schemas — CUE schema system

OVERVIEW

  Schemas are CUE files that define the structure and validation
  rules for data in the catalog. They live in a git-tracked
  repository under ~/.pudl/schema/.

SCHEMA NAMING

  Schemas follow the CUE convention: package.#Definition

    aws.#EC2Instance
    k8s.#Deployment
    pudl/core.#Item        (the catchall)

  Normalized form: "pkg.#Name" — use this everywhere.

SCHEMA LOCATIONS

  ~/.pudl/schema/           Global schema repository
  .pudl/schema/             Repo-local schemas (shadows global)

COMMANDS

  pudl schema list                   List all schemas by package
  pudl schema show <name>            Display schema CUE source
  pudl schema new <name>             Generate schema from imported data
  pudl schema add <name> <file>      Add a schema file
  pudl schema edit <name>            Edit schema in $EDITOR
  pudl schema reinfer                Re-run inference on all entries

SCHEMA INFERENCE

  When data is imported, pudl runs a multi-stage inference:

  1. Structural heuristics — field names, shapes, nesting
  2. CUE unification — test data against each candidate schema
  3. Best-match selection — most specific schema that validates

  The inference result is stored on the catalog entry. Re-inference
  can be triggered with 'pudl schema reinfer' after schema changes.

VERSION CONTROL

  The schema repository is git-tracked:

  pudl schema status       Show uncommitted schema changes
  pudl schema commit       Commit schema changes
  pudl schema log          Show schema change history

  This gives you a full audit trail of schema evolution.

MODULES

  pudl supports CUE module dependencies:

  pudl module list         List current dependencies
  pudl module add <mod>    Add a third-party module
  pudl module tidy         Fetch and update dependencies
  pudl module info         Show module information

SEE ALSO

  pudl guide import        How schemas are assigned during import
  pudl guide definitions   Named instances of schemas
`)
}

func printGuideFacts() {
	fmt.Print(`pudl guide facts — bitemporal fact store

OVERVIEW

  The fact store holds structured assertions about the world. Each
  fact has two time dimensions:

    valid_start / valid_end      When the fact was true in reality
    tx_start / tx_end            When pudl learned/forgot the fact

  This lets you ask both "what was true at time X" and "what did
  we believe at time X."

WRITING FACTS — ONE DOOR

  pudl facts add --relation <rel> --args '<json-object>'   The canonical write.
  pudl facts observe "<text>" --kind <kind> --scope <s>    Sugar for observations.

  Every fact write goes through facts add (observe is just the ergonomic
  observation path). Import data with 'pudl import'; bridge to mu with
  'pudl mu …' — those are different doors, not fact writes.

  Known agent relations are validated on write against their built-in schema:

    observation   kind ∈ {fact, obstacle, pattern, antipattern, suggestion,
                  bug, opportunity}; scope is repo:path
    feedback      verdict ∈ {helpful, harmful, neutral}; target = fact/rule id

  Examples:
    pudl facts observe "auth module has no rate limiting" \
      --kind suggestion --scope myapp:pkg/auth --source claude-code
    pudl facts add --relation feedback \
      --args '{"target":"<fact-id>","verdict":"helpful","source":"claude-code"}'

QUERYING FACTS

  pudl facts list                           List all current facts
  pudl facts list --relation observation    Filter by relation
  pudl facts list --source claude-code      Filter by source
  pudl facts show <id>                      Full fact details
  pudl facts stats                          Aggregate statistics

FACT LIFECYCLE

  Facts are append-only. You never update a fact — instead:

  pudl facts promote <id> --to reviewed     Advance maturity
                               (raw → reviewed → promoted | rejected)
  pudl facts promote <id> --to promoted --rule <ref>
  pudl facts retract <id>      Mark as no longer asserted
                               (we were wrong about this)
  pudl facts invalidate <id>   Mark as no longer true
                               (it was true but isn't anymore)

  promote, retract, and invalidate set tx_end/valid_end and append a new
  version, preserving the original assertion for historical queries.

TIME-TRAVEL QUERIES

  pudl facts list --as-of-valid "2025-01-15T00:00:00Z"
  pudl facts list --as-of-tx "2025-01-15T00:00:00Z"

  --as-of-valid: "What was true at this time?"
  --as-of-tx:    "What did we believe at this time?"

PULLING RELATED FACTS

  pudl pull --scope <scope>          All facts for a scope
  pudl pull --entity <entity>        All facts for an entity
  pudl pull --relation <relation>    All facts of a relation type

TABLES

  facts           Append-only, full bitemporal history
  current_facts   Materialized view of currently-valid facts
                  (synced transactionally by AddFact/RetractFact/InvalidateFact)

SEE ALSO

  pudl guide datalog       Derive new facts with rules
  pudl guide agents        Conventions for agent-recorded observations
`)
}

func printGuideDatalog() {
	fmt.Print(`pudl guide datalog — query engine and rules

OVERVIEW

  pudl includes a Datalog query engine that derives new facts from
  existing ones using declarative rules. Rules are CUE files that
  compile to parameterized SQL queries.

QUERYING

  pudl query <relation>                   Query derived facts
  pudl query <relation> --field=value     Filter results
  pudl query <relation> --json            JSON output

  Example:
    pudl query stale-observations --age=7d

RULES

  Rules are CUE files stored in:

    ~/.pudl/schema/pudl/rules/       Global rules
    .pudl/schema/pudl/rules/         Repo-local (shadows global)

  Install a rule:
    pudl rule add myrule.cue              Install to repo
    pudl rule add myrule.cue --global     Install globally

HOW RULES WORK

  Each rule is a CUE field with a head (the derived relation) and a
  body (conditions over existing facts). Arguments are named; variables
  use the $-prefix convention:

    package rules

    stale_item: {
        head: { rel: "stale_item", args: { entity: "$E", age: "$A" } }
        body: [
            { rel: "observation", args: { entity: "$E", time: "$T" } },
            { rel: "older_than",  args: { time: "$T", age: "$A" } },
        ]
    }

  The SQL compiler translates each body atom into a self-join on
  current_facts with json_extract() for argument access. Shared
  $Variables become equi-join conditions.

EVALUATION MODES

  Non-recursive rules: compiled directly to SQL, evaluated once.
  Recursive rules: semi-naive fixpoint via SQLite temp tables.

  The engine automatically partitions rules into recursive and
  non-recursive sets.

SQL COMPILATION

  Each body atom becomes a self-join on current_facts:

    { rel: "observation", args: { entity: "$E", desc: "$D" } }
    →
    SELECT json_extract(t0.args, '$.entity') AS entity,
           json_extract(t0.args, '$.desc')   AS desc
    FROM current_facts t0
    WHERE t0.relation = 'observation'

  Shared variables across atoms produce equi-joins.

CATALOG AS A RELATION

  The catalog is exposed as the built-in 'catalog_entry' relation, so
  rules can join facts against catalog data (fields: id, schema, origin,
  format, status, entry_type, definition, resource_id, ...):

    owned: {
        head: { rel: "owned", args: { id: "$I", team: "$T" } }
        body: [
            { rel: "catalog_entry", args: { id: "$I", origin: "$O" } },
            { rel: "team_owns",     args: { origin: "$O", team: "$T" } },
        ]
    }

  catalog_entry is join-only (use it in a rule body, not as a direct
  query target) and reserved (facts cannot be asserted under that name).
  To list catalog entries directly, use 'pudl list'.

SEE ALSO

  pudl guide facts         The underlying fact store
  pudl guide pith          Stack-based programs (alternative to Datalog)
`)
}

func printGuideDefinitions() {
	fmt.Print(`pudl guide definitions — named schema instances

OVERVIEW

  Definitions are named instances of schemas with concrete
  configuration values. They declare the desired state of
  resources and can wire to other definitions via sockets.

COMMANDS

  pudl definition list                 List all definitions
  pudl definition show <name>          Show definition details
  pudl definition validate             Validate all definitions
  pudl definition graph                Show dependency graph

WHAT DEFINITIONS DO

  A definition binds a schema to concrete values:

    package definitions

    nginx_conf: file.#Config & {
        path:    "/etc/nginx/nginx.conf"
        content: "server { listen 80; }"
        mode:    "0644"
    }

  Definitions are:
  - Named (globally unique within the workspace)
  - Typed (must satisfy their schema)
  - Composable (can reference other definitions via sockets)
  - Observable (pudl can check if reality matches)

VALIDATION

  pudl definition validate checks all definitions against their
  schemas using CUE unification. Errors report which fields
  fail validation and why.

DEPENDENCY GRAPH

  pudl definition graph renders the dependency graph between
  definitions. Definitions reference each other through socket
  fields, creating a DAG of desired state.

SEE ALSO

  pudl guide schemas       The schema system definitions build on
  pudl guide drift         Detecting divergence from desired state
  pudl guide mu            Using mu to converge drifted definitions
`)
}

func printGuideDrift() {
	fmt.Print(`pudl guide drift — detecting state divergence

OVERVIEW

  Drift detection compares declared state (definitions) against
  observed reality (imported data, facts). When they diverge,
  pudl reports the drift so you can converge.

COMMANDS

  pudl drift check [<definition>]      Check for drift
  pudl drift report                    Generate a drift report
  pudl status                          Show convergence status

WORKFLOW

  1. Define desired state (CUE definitions)
  2. Import or observe actual state
  3. Run drift check to compare
  4. Export actions for mu to converge (or fix manually)

EXPORTING ACTIONS

  pudl mu export-actions --definition <name>

  Generates a mu.json config that mu can execute to converge
  the drifted resource. See 'pudl guide mu' for the full loop.

SEE ALSO

  pudl guide definitions   Declaring desired state
  pudl guide mu            The ACUTE convergence loop
`)
}

func printGuidePith() {
	fmt.Print(`pudl guide pith — stack-based programs against the data lake

OVERVIEW

  pith is a concatenative (stack-based) VM. Programs are JSON
  arrays of words interpreted against a stack. pudl and mu share
  the same VM — programs written for one transfer to the other.

RUNNING PROGRAMS

  pudl exec '<program>'                    Run a program
  pudl exec --trace '<program>'            Run with trace output
  echo '<program>' | pudl exec --stdin     Read program from stdin

PROGRAM SYNTAX

  Programs are JSON arrays. Each element is:

    word       dispatched immediately (e.g. "dup", "add")
    literal    pushed to stack (numbers, booleans)
    'string    string literal (single-quote prefix avoids dispatch)
    [...]      quotation (pushed for deferred execution)

  Example:
    ["'hello", "'world", "concat"]    → "helloworld"
    [2, 3, "add"]                     → 5
    [[1,2,3], ["dup", "mul"], "map"]  → [1,4,9]

CORE VOCABULARY

  Stack:      dup drop swap over nip rot tuck 2dup 2drop
  Combs:      apply dip keep bi bi* bi@ each map filter reduce
  Control:    if when unless
  Objects:    get set has? keys values path pick omit merge
  Compare:    eq neq lt gt lte gte
  Logic:      and or not null?
  Arith:      add sub mul div mod
  Strings:    concat len split
  Sequences:  group-by flatten

DRIVER WORDS (pudl-specific)

  catalog/query    Query the catalog
  schema/match     Match data against schemas
  fact/list        Query the fact store

  These are registered by pudl at startup and are not available
  in mu's pith environment (mu has its own driver words).

HEREDOC INPUT

  For complex programs, use heredoc syntax:

    pudl exec --stdin <<'EOF'
    [
      "'hello",
      "'world",
      "concat"
    ]
    EOF

SEE ALSO

  pudl guide datalog       Alternative query approach (declarative)
  pudl guide mu            mu's pith integration (plan/transform/execute)
`)
}

func printGuideMu() {
	fmt.Print(`pudl guide mu — how pudl and mu work together

OVERVIEW

  pudl and mu are decoupled tools that communicate through mu.cue.

    pudl: defines desired state, observes actual state, computes drift.
    mu:   takes desired-state targets and converges them using plugins.

  Neither tool imports or depends on the other.

THE ACUTE LOOP

  Assess → Converge → Unify → Test → Emit

  1. ASSESS: Import actual state, check drift

     pudl import --path state.json
     pudl drift check nginx_conf

  2. CONVERGE: Export actions and run mu

     pudl mu export-actions --definition nginx_conf > converge.json
     mu build --config converge.json //nginx_conf

  3. UNIFY: Re-observe and verify

     mu observe --ndjson //nginx_conf | pudl import --stdin
     pudl drift check nginx_conf    # should report no drift

OBSERVATION PIPELINE

  mu observe --ndjson <targets> | pudl import --stdin

  mu's observe output streams records with _schema fields. pudl
  routes each record by schema to the appropriate definition.

INGESTING MU ARTIFACTS

  pudl mu ingest-manifest <manifest.json>  Ingest build manifests
  pudl mu ingest-observe <results.json>    Ingest observe results

  These commands understand mu's output formats natively.

SHARED PITH VM

  pudl and mu share the same pith VM and data model (JSON on stack).
  pudl registers read-only words (catalog/query, fact/list). mu
  registers effectful words (http/get, exec/run, action/emit).

  An agent that writes pith programs for pudl can immediately
  write programs for mu — only the driver vocabulary differs.

SEE ALSO

  pudl guide drift         Drift detection (the "Assess" step)
  pudl guide pith          Pith VM shared between pudl and mu
  mu guide pudl            mu's perspective on the integration
`)
}

func printGuideAgents() {
	fmt.Print(`pudl guide agents — conventions for AI agents

OVERVIEW

  pudl is designed for both human and agent use. This guide covers
  conventions and best practices for AI agents working with pudl.

  See also: 'pudl prime' outputs a structured prompt you can include
  in agent configuration files (CLAUDE.md, etc.).

RECORDING OBSERVATIONS

  Always pass --source with your agent name:

    pudl facts observe "auth module lacks rate limiting" \
      --kind suggestion \
      --scope myapp:pkg/auth \
      --source claude-code

  This makes observations attributable and filterable.

SCOPE FORMAT

  Use repo:path format for globally unambiguous scoping:

    pudl:internal/database
    myapp:pkg/auth
    infra:terraform/vpc

  Consistent scoping makes observations joinable across repos.

MACHINE-READABLE OUTPUT

  Pass --json on any command for structured output:

    pudl list --json
    pudl facts list --json
    pudl query stale-items --json
    pudl status --json

ID FORMAT

  IDs are content-addressed SHA256 displayed as proquints
  (e.g. "babam-babam"). Short prefixes work when unambiguous.

TEMPORAL QUERIES

  Query historical state with time-travel flags:

    pudl facts list --as-of-valid "2025-01-15T00:00:00Z"
    pudl facts list --as-of-tx "2025-01-15T00:00:00Z"

  --as-of-valid: "What was true at this time?"
  --as-of-tx:    "What did we believe at this time?"

SCHEMA INFERENCE

  Schema inference is automatic on import. You usually don't need
  --schema unless forcing a specific classification.

RECOMMENDED WORKFLOWS

  Explore a codebase and record findings:
    1. Analyze code
    2. pudl facts observe "<finding>" --kind <kind> --scope <repo:path>
    3. pudl facts list --source claude-code  (review what you've recorded)

  Query existing knowledge:
    1. pudl pull --scope <repo:path>  (all facts for a scope)
    2. pudl query <relation>         (derived facts from rules)

  Import external data:
    1. curl ... | pudl import --stdin --format json
    2. pudl list --schema <name>     (verify import)
    3. pudl show <id>                (inspect details)

INTEGRATING WITH AGENT CONFIG

  Add this line to your CLAUDE.md or agent config:

    Run 'pudl prime' to learn how to use the pudl data lake CLI.

  Or for the full reference:

    Run 'pudl guide overview' for a quick introduction to pudl.
`)
}
