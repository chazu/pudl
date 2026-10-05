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
			fmt.Fprintf(errw(), "pudl guide: unknown topic %q\n", topic)
			fmt.Fprintln(errw(), "Run 'pudl guide' for a list of topics.")
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
	"overview":        printGuideOverview,
	"import":          printGuideImport,
	"schemas":         printGuideSchemas,
	"facts":           printGuideFacts,
	"datalog":         printGuideDatalog,
	"models":          printGuideModels,
	"mu":              printGuideMu,
	"agents":          printGuideAgents,
	"troubleshooting": printGuideTroubleshooting,
}

func init() {
	rootCmd.AddCommand(guideCmd)
}

func printGuideIndex() {
	fmt.Fprint(outw(), `pudl guide — quick-reference for agents and humans

Start here if you're new to pudl:

  pudl guide overview       What pudl is, the mental model, and where to go next

Data management:

  pudl guide import         Importing data: formats, schemas, wildcards, stdin
  pudl guide schemas        CUE schema system: inference, authoring, versioning

Reasoning and state:

  pudl guide facts          Bitemporal assertions, retraction, and historical queries
  pudl guide datalog        Datalog query engine: rules, recursive evaluation

Convergence:

  pudl guide models         #SystemModels: list, show, run, converge
  pudl guide mu             How pudl drives mu (the ACUTE loop)

For agents:

  pudl guide agents         Conventions, best practices, and tips for AI agents
  pudl guide troubleshooting Common failure modes and recovery commands
`)
}

func printGuideOverview() {
	fmt.Fprint(outw(), `pudl guide overview — what pudl is, in 60 seconds

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
  - models           #SystemModels: declared shape + populate/converge
  - fact store       bitemporal assertions (valid-time + transaction-time)
  - datalog rules    CUE-defined rules for derived queries
  - workspace        one durable state root: repo .pudl/ or global ~/.pudl/

THE DAY-TO-DAY VERBS

  pudl import --path <file>   Import data (auto-detects format + schema).
  pudl list                   Browse catalog entries.
  pudl show <id>              Inspect an entry's content and metadata.
  pudl facts list --relation depends   Query assertions in the fact store.
  pudl query <relation>       Run Datalog queries over derived facts.
  pudl facts add --relation <name> --args '<json-object>'   Record an assertion.
  pudl run <model>            Run a #SystemModel (observe-only, or --converge).
  pudl status                 Show recorded convergence status.
  pudl doctor                 Health check the workspace.

WHERE DATA LIVES

  Inside a repo initialized with pudl init, all durable state is local:
  .pudl/data/sqlite/catalog.db    Catalog, reports, approvals, and facts.
  .pudl/data/{raw,metadata}/      Imported content and provenance.
  .pudl/schema/                   Local CUE module, built-ins, models, rules.
  .pudl/config.yaml               Local path configuration.
  .pudl/workspace.cue             Workspace identity and policy.

  Outside a repo workspace the same layout lives under ~/.pudl/.

WHAT TO READ NEXT

  Getting started:     pudl guide import → pudl guide schemas
  Recording state:     pudl guide facts → pudl guide datalog
  Convergence:         pudl guide models → pudl guide mu
  Integration with mu: pudl guide mu
  For AI agents:       pudl guide agents
`)
}

func printGuideImport() {
	fmt.Fprint(outw(), `pudl guide import — importing data into the catalog

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

STDIN SUPPORT

  Pipe data directly into pudl:

    curl -s https://api.example.com/data | pudl import --path - --format json
    cat <<'EOF' | pudl import --path - --format json
    {"name": "test", "value": 42}
    EOF

SCHEMA INFERENCE

  On import, pudl automatically:

  1. Detects the data format
  2. Infers a CUE schema from the data structure
  3. Matches against existing schemas (exact or structural)
  4. Assigns the best-matching schema or the catchall

  Use --schema to force a specific schema assignment.

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

  --path <path>       File path or glob pattern; '-' reads stdin
  --format <fmt>      Force format (json, yaml, csv, ndjson)
  --schema <name>     Force schema assignment
  --json              Output results as JSON
`)
}

func printGuideSchemas() {
	fmt.Fprint(outw(), `pudl guide schemas — CUE schema system

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
  pudl schema new --from <id> --path <package>/#<Definition>
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
  pudl guide models        #SystemModels built on schemas
`)
}

func printGuideFacts() {
	fmt.Fprint(outw(), `pudl guide facts — generic bitemporal assertions

WRITE AND QUERY

  pudl facts add --relation depends --args '{"from":"api","to":"database"}'
  pudl facts add --relation config --args '{"key":"timeout","value":30}' --source operator
  pudl facts list --relation depends --json
  pudl facts search "timeout" --relation config --json
  pudl facts show <id>

  Args are arbitrary JSON objects. Use --schema to validate against an authored
  CUE definition; no relation has a built-in maturity or scoring policy.

LIFECYCLE AND HISTORY

  pudl facts retract <id>      Withdraw an incorrect assertion
  pudl facts invalidate <id>   Record that a previously true assertion expired
  pudl facts list --relation config --as-of-valid "2026-04-01T14:30:00Z"
  pudl facts list --relation config --as-of-tx "2026-04-01T14:30:00Z"

  valid_start / valid_end describe when a fact was true in reality.
  tx_start / tx_end describe when PUDL held that belief.
  Historical assertions remain retained; current_facts and its full-text index
  track currently valid, non-retracted facts transactionally.

SEE ALSO

  pudl guide datalog       Joins and recursive rules over facts and catalog data
`)
}

func printGuideDatalog() {
	fmt.Fprint(outw(), `pudl guide datalog — query engine and rules

OVERVIEW

  pudl includes a Datalog query engine that derives new facts from
  existing ones using declarative rules. Rules are CUE files that
  compile to parameterized SQL queries.

QUERYING

  pudl query <relation>                   Query derived facts
  pudl query <relation> field=value       Filter results
  pudl query <relation> --json            JSON output

  Example:
    pudl query stale-observations age=7d

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
`)
}

func printGuideModels() {
	fmt.Fprint(outw(), `pudl guide models — declaring and running #SystemModels

OVERVIEW

  A #SystemModel packages a system's shape, how to populate it (observe
  live state), optional desired state, and how to converge — in one
  declaration. 'pudl run <model>' drives it through the ACUTE cycle:
  populate → drift → checks → report (and, with --converge, applies
  changes via mu).

COMMANDS

  pudl model list                      List registered models (+ last-run status)
  pudl model show <name>               Show populate/converge/desired/checks
  pudl model validate <name>           Structurally validate without running
  pudl run <name>                      Observe-only run (populate → drift → checks)
  pudl run <name> --converge           Close drift (mutates the target via mu)
  pudl run set <models...>             Observe an exact dependency set
  pudl run set <models...> --converge  Plan the whole set, then mutate
  pudl run report [id]             Read a durable run-set report
  pudl run resume|reject <id>      Decide a pending exact plan
  pudl status                          Show recorded convergence status

WHAT A MODEL DECLARES

  A model is a CUE definition inheriting #SystemModel, registered in the
  schema repo (project .pudl/schema shadows global ~/.pudl/schema):

    githubChazu: #SystemModel & {
        name: "github-chazu"
        populate: { eweSource: "github-chazu/populate.cue", outputs: ["repos.json"] }
        // optional: desired: [...], converge: { plugin: "k8s" }, checks: [...]
    }

  Register with 'pudl schema add'; resolve and run it by name (its 'name'
  field or short definition name).

SCAFFOLD FIRST

  pudl model new pods --populate plugin:k8s --input namespace=default
  pudl model show pods --json
  pudl run --populate plugin:k8s --input inventory='{"kinds":["pods"]}'

  The ad-hoc form writes no model definition and is observe-only. For a durable
  model, edit the path printed by 'model new' rather than authoring registration
  boilerplate by hand. Use 'pudl run report [run-id]' after a run; use
  '--require-approval' plus 'run resume'/'run reject' before convergence.

CROSS-MODEL VALUES

  Required scalar inputs can bind to a producer observation. Both the consumer
  input and source schema field must declare @pudl(binding=plain). 'run set'
  runs exactly the named models, rejects omitted producers/cycles in preflight,
  orders producers first, and pins their successful snapshots. It never expands
  the set implicitly.

  Sealed values stay inside mu's provider channel. PUDL-generated targets use
  strict action routing, and a run-set that can write a sealed output always
  pauses for exact-plan approval. Invalid or ambiguous claims fail during
  whole-set planning before mutation or provider traffic. Each approved apply
  passes mu the raw plan digest; mu compares before provider access and executes
  that same in-memory graph.

SEE ALSO

  pudl guide schemas       The schema system models build on
  pudl guide mu            How pudl drives mu (the ACUTE loop)
`)
}

func printGuideMu() {
	fmt.Fprint(outw(), `pudl guide mu — how pudl and mu work together

OVERVIEW

  pudl and mu are decoupled tools. pudl declares desired state, observes
  actual state, and computes drift; mu takes desired-state targets and
  converges them using plugins. Neither imports the other — they
  communicate through a generated mu.cue and JSON results.

THE ACUTE LOOP (driven by 'pudl run')

  populate → drift → checks → report, repeated under --converge until
  observed == desired (or an iteration cap):

  1. populate: pudl runs 'mu observe' (or an ewe fetch) and ingests the
               records into the catalog.
  2. drift:    pudl compares the model's desired state against the latest
               observation.
  3. converge: (--converge) pudl renders desired → sources and runs
               'mu build'; the plugin reconciles. pudl re-observes to
               verify, looping to a fixed point.

  pudl run github-chazu                 # observe-only
  pudl run k8sPolicy --converge         # close drift via mu
  pudl run set network k8sPolicy        # exact producer/consumer set

VALUE ROUTING

  Plain scalar bindings come from PUDL catalog snapshots and carry durable
  producer/run/snapshot/path evidence. Sealed values stay in mu's provider
  channel and PUDL records only schemes and fingerprints. Generated targets use
  strict per-action claims. Sealed-output run-sets pause for exact approval;
  resume rebuilds the plan before producer-first execution.

INGESTING MU RESULTS

  pudl mu ingest-observe  <results.json>   Store observe results as live state
  pudl mu ingest-manifest <manifest.json>  Record a build manifest

  These understand mu's output formats natively. 'pudl run' calls them
  for you, but they're also usable standalone.

DISTRIBUTION

  mu plugins are published and discovered through mu's OCI path:
    mu plugin push <name>
    mu plugin list --remote
    mu plugin info <name> --json

  PUDL schemas and model contracts remain CUE modules:
    pudl module add <module@version>
    pudl module tidy

  Compose the two by declaring the resolved mu plugin in a model's plugins:
  block and keeping the PUDL schema/model in the CUE module. Do not create a
  second registry format for a plugin bundle.

SEE ALSO

  pudl guide models        Declaring and running #SystemModels
  pudl guide datalog       Querying the catalog and derived facts
`)
}

func printGuideAgents() {
	fmt.Fprint(outw(), `pudl guide agents — operate models and inspect evidence

START WITH THE PROJECT

  pudl model list --json
  pudl model show <name> --json
  pudl run <name>
  pudl run report --json

  Existing repository state lives under .pudl/. Create it with pudl init if
  needed; use --global explicitly for global initialization.

EXECUTION SCOPE

  pudl run set <producer> <consumer>   Observe exactly the named set
  pudl run set <models...> --converge --require-approval
  pudl run resume <operation-id>
  pudl run reject <operation-id>

  Ordinary runs observe. --converge enables mutation through Mu. Missing
  producers are not started implicitly. Set approval covers an exact plan;
  changed plans are rejected. Sealed values stay inside Mu's provider path.

INTERPRET THE RESULT

  Operation completion, resource conformity, check results, and verification
  are distinct. Read the stored report by ID for provenance and findings.
  Stored replay is not fresh live verification. Uncertain mutation outcomes
  require verification, and partial scope cannot prove a whole model clean.

DATA AND RULES

  pudl import --path <file>
  pudl list --json
  pudl show <id> --raw
  pudl query <relation> key=value --json
  pudl facts add --relation <name> --args '<json-object>' --source <origin>

  Schema inference is automatic. Generic fact writes should name their source;
  use explicit --schema validation when an assertion has an authored contract.
  facts list --as-of-valid/--as-of-tx supports historical evidence queries.

DISCOVERY

  pudl help <command> --json
  pudl guide <topic>
  pudl prime
`)
}

func printGuideTroubleshooting() {
	fmt.Fprint(outw(), `pudl guide troubleshooting — diagnose the shipped path

DISCOVER THE ACTUAL SURFACE

  pudl help --json                 command tree and flags
  pudl model show <name> --json
  mu plugin info <name> --json     plugin capabilities/config schema

RUN DIAGNOSTICS

  pudl run report [<run-id>] --json   retrieve the most recent/stored report
  pudl status                         model verdicts
  pudl list --artifacts               run and manifest artifacts

COMMON CASES

  No model found:
    pudl model list
    pudl model new <name> --populate plugin:<name>

  Plugin cannot be resolved:
    mu plugin list --cached
    mu plugin info <name> --json
    pudl run <model> --mu-root <directory-containing-mu.cue>

  Observe records have an unexpected schema:
    inspect the plugin's _schema field, then use `+"`pudl mu ingest-observe`"+`
    with the current schema repository. PUDL only persists references present
    in the loaded schema namespace; unresolved declarations fall back safely.

  Convergence stopped after mutation:
    inspect `+"`pudl run report <run-id> --json`"+`. A needs_verification/unknown
    result means receipt or re-observation proof was incomplete; do not blindly
    re-apply.

  Approval pending:
    pudl run report <run-id> --json
    pudl run resume <run-id>       # approve and execute
    pudl run reject <run-id>       # terminal no-op
`)
}
