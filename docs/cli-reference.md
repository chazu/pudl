# CLI Reference

All commands support `--json` for machine-readable output and `--help` for inline documentation.

## Workspace

### `pudl init`

Initialize the PUDL workspace at `~/.pudl/`.

Creates the configuration file, data directories, and a git-tracked schema repository with bootstrap schemas. Safe to run multiple times -- skips if already initialized.

```bash
pudl init
pudl init --force  # Reinitialize (preserves existing data)
```

### `pudl config`

View or modify configuration.

```bash
pudl config                          # Show current configuration
pudl config set data_path ~/my-data  # Change a setting
pudl config reset                    # Reset to defaults
```

### `pudl version`

Show version, commit, and build date.

### `pudl doctor`

Run health checks on the PUDL workspace.

### `pudl setup`

Set up shell integration (aliases, completion, helper functions).

```bash
pudl setup                     # Auto-detect shell and install
pudl setup --shell bash        # Force bash setup
pudl setup --dry-run           # Show what would be added
pudl setup --uninstall         # Remove PUDL integration
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--shell` | Target shell: `bash`, `zsh`, `fish` (auto-detected if omitted) |
| `--dry-run` | Preview changes without modifying config files |
| `--uninstall` | Remove PUDL shell integration from all detected shells |

### `pudl completion`

Generate shell completion scripts.

```bash
pudl completion bash
pudl completion zsh
pudl completion fish
```

## Data Import

### `pudl import`

Import data files with automatic format detection, schema inference, and provenance tracking.

```bash
pudl import --path <file>
pudl import --path <file> --origin <source>
pudl import --path <file> --schema <schema-name>
pudl import --path <file> --format <format>
pudl import --path "*.json"                       # Wildcard batch import
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--path` | File to import (required). Supports wildcards. Use `-` for stdin. |
| `--origin` | Override automatic origin detection |
| `--schema` | Specify schema explicitly (skip inference) |
| `--format` | Specify format when reading from stdin (`json`, `yaml`, `csv`, `ndjson`) |
| `--streaming-memory` | Memory limit in MB for streaming (default: 100) |
| `--streaming-chunk-size` | Chunk size in MB for streaming (default: 0.016) |

**Behavior:**

- Duplicate files (same content hash) are skipped automatically
- NDJSON files are split into collections with individual items
- JSON API wrapper responses are detected and unwrapped (score >= 0.50)
- Format is detected from extension and content analysis
- Origin is inferred from filename patterns

**Debug mode:** Set `PUDL_DEBUG=1` for detailed error output.

## Data Discovery

### `pudl list`

Query and filter catalog entries.

```bash
pudl list
pudl list --schema aws/ec2.#Instance
pudl list --origin k8s --format yaml
pudl list --sort-by size --reverse --limit 10
pudl list --collections-only
pudl list --collection-id my-inventory --schema aws/ec2.#Instance
pudl list --fancy  # Interactive TUI
```

**Filter flags:**

| Flag | Description |
|------|-------------|
| `--schema` | Filter by schema name (partial match, case-insensitive) |
| `--origin` | Filter by origin (partial match, case-insensitive) |
| `--format` | Filter by format (`json`, `yaml`, `csv`, `ndjson`) |
| `--collections-only` | Show only collection entries |
| `--items-only` | Show only individual item entries |
| `--collection-id` | Show items from a specific collection |
| `--item-id` | Show a specific item by its item ID |

**Display flags:**

| Flag | Description |
|------|-------------|
| `--verbose` | Show file paths, identity info, and summary statistics |
| `--limit` | Maximum number of entries to display |
| `--sort-by` | Sort field: `timestamp`, `size`, `records`, `schema`, `origin`, `format` |
| `--reverse` | Reverse sort order |
| `--page` | Page number for pagination |
| `--per-page` | Entries per page |
| `--fancy` | Launch interactive TUI (bubbletea) |

### `pudl show <id>`

Inspect a specific catalog entry. Accepts proquint IDs (`mivof-duhij`) or full hex hashes.

```bash
pudl show mivof-duhij
pudl show mivof-duhij --raw         # Show raw data content
pudl show mivof-duhij --metadata    # Show import metadata
```

### `pudl export`

Export data in various formats.

```bash
pudl export --id babod-fakak                    # Export single entry by proquint ID
pudl export --schema aws.#EC2Instance           # Export all EC2 instances
pudl export --origin k8s-pods --format yaml     # Export K8s pods as YAML
pudl export --id babod-fakak --output out.json  # Export to file
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--id` | Export entry by proquint ID |
| `--schema` | Export entries matching schema |
| `--origin` | Export entries from origin |
| `--format` | Output format: `json`, `yaml`, `csv`, `ndjson` (default: `json`) |
| `--output` / `-o` | Output file (default: stdout) |
| `--pretty` | Pretty-print output (default: true) |

At least one of `--id`, `--schema`, or `--origin` is required.

## Data Management

### `pudl delete <id>`

Remove a catalog entry and its associated files (raw data + metadata).

```bash
pudl delete mivof-duhij              # With confirmation prompt
pudl delete mivof-duhij --force      # Skip confirmation
pudl delete govim-nupab --cascade    # Delete collection + all items
pudl delete mivof-duhij --json       # JSON output for scripting
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--force` | Skip confirmation prompt |
| `--cascade` | Delete collection and all its child items |
| `--json` | Output results as JSON |

Collections with items cannot be deleted without `--cascade`. Individual items can always be deleted.

### `pudl validate`

Validate catalog data against assigned CUE schemas.

```bash
pudl validate --entry babod-fakak      # Validate a specific entry by proquint
pudl validate --all                     # Validate all catalog entries
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--entry` | Validate a specific entry by proquint ID |
| `--all` | Validate all catalog entries |

Exactly one of `--entry` or `--all` is required.

### `pudl verify`

Verify that schema inference is a fixed point for all catalog entries. Re-runs inference on every entry and confirms each still resolves to the same schema it was originally assigned.

```bash
pudl verify
```

Any mismatch indicates drift between stored assignments and current inference rules.

## Schema Management

### `pudl schema list`

List available schemas.

```bash
pudl schema list
pudl schema list --package aws
pudl schema list --package k8s --verbose
```

### `pudl schema add <name> <file>`

Add a CUE schema file to the repository.

```bash
pudl schema add aws.rds-instance my-rds-schema.cue
pudl schema add custom.api-response api-schema.cue
```

### `pudl schema new`

Generate a CUE schema from imported data.

```bash
pudl schema new --from <id> --path <package>#<Definition>
pudl schema new --from mivof-duhij --path mypackage/#MyResource
pudl schema new --from mivof-duhij --path mypackage/#MyResource --infer status=enum
pudl schema new --from govim-nupab --collection --path mypackage/#MyItem
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--from` | Catalog entry ID to generate schema from |
| `--path` | Target schema path (`package/#Definition`) |
| `--collection` | Generate schema for collection items (not the wrapper) |
| `--infer <field>=enum` | Infer field as enum from observed values |

### `pudl schema show <name>`

Print schema file contents to stdout.

### `pudl schema edit <name>`

Open a schema file in `$EDITOR`.

### `pudl schema validate`

Validate CUE schema files for correctness.

### `pudl schema reinfer`

Re-run schema inference on all existing catalog entries. Use after adding or modifying schemas to reclassify data.

```bash
pudl schema reinfer
```

### `pudl schema migrate`

Migrate schema names to canonical `<package-path>.#<Definition>` format.

### Schema Version Control

The schema directory is a git repository:

```bash
pudl schema status                     # Show uncommitted changes
pudl schema commit -m "Add RDS schema" # Commit changes
pudl schema log                        # Show commit history
pudl schema log --verbose              # Detailed history
```

## Schema Catalog

### `pudl catalog`

Display the schema catalog -- a central inventory of all registered schema types with their metadata.

```bash
pudl catalog              # List all registered types
pudl catalog --verbose    # Show identity fields, tracked fields, etc.
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--verbose` / `-v` | Show additional metadata fields (identity_fields, tracked_fields) |

## Models

A model is a registered `#SystemModel` describing a `desired` state and the sources that populate observed state. Catalog rows carry a `target` — the mu target / run target that produced them (e.g. `//models/<name>`, a populate phase `//models/<name>:populate`, or a standalone observe like `home/odroid`); status is recorded per target. Drift detection is a phase of `pudl run`, not a standalone command.

### `pudl model list`

List registered `#SystemModel`s along with the status of each model's last run.

```bash
pudl model list
pudl model list --json
```

### `pudl model show <name>`

Display detailed information about a single model.

```bash
pudl model show my_model
pudl model show my_model --json
```

### `pudl model validate <name>`

Validate a model against its schema.

```bash
pudl model validate my_model
pudl model validate my_model --json
```

## Convergence

### `pudl run <name>`

Run a model's ACUTE loop. By default this is observe-only: it populates observed state, detects drift, runs checks, and prints a report without making changes.

With `--converge` it closes drift: pudl renders the desired state to sources and the mu plugin reconciles the live system toward it.

```bash
pudl run my_model                 # observe-only: populate -> drift -> checks -> report
pudl run my_model --converge      # close drift (render desired -> sources, mu reconciles)
pudl run my_model --only foo      # restrict to a single definition
pudl run my_model --dry-run       # show planned actions without applying
```

> **Host credentials for converge plugins.** mu runs converge actions with a
> hermetic environment — it does **not** inherit your shell's `HOME` or
> `KUBECONFIG`. A plugin that needs host credentials must get them through the
> model's `converge.input`, since pudl carries no domain knowledge to inject
> them. For the **k8s** plugin set `input.kubeconfig` to an absolute path:
>
> ```cue
> converge: #PluginPlan & {
>     plugin: "k8s"
>     input: {namespace: "...", context: "...", kubeconfig: "/abs/path/kubeconfig"}
> }
> ```
>
> Without it, apply fails with `context "…" does not exist` (kubectl falls back
> to an empty config because it can't find `~/.kube/config`). Observe is
> unaffected — it runs inside the plugin process, which keeps the full env.

**Flags:**

| Flag | Description |
|------|-------------|
| `--converge` | Close drift instead of only observing |
| `--only` | Restrict the run to a single definition |
| `--dry-run` | Show planned actions without applying them |
| `--max-iters` | Maximum convergence iterations |
| `--from-catalog` | Force inventory drift from the catalog (override; inventory observers — EweTarget or `#PluginObserve` `differential: false` — auto-route here) |
| `--mu-root` | Path to the mu workspace root used for reconciliation |

### `pudl status [target]`

Read convergence status from the catalog. A model run records its verdict on the instance row, keyed by target `models/<name>`. With no argument, reports status for all targets; with a target name, reports just that one.

```bash
pudl status
pudl status models/my-model
```

Statuses: `unknown`, `clean`, `drifted`, `converging`, `failed`. Lifecycle:
`drifted → converging` (apply, via `ingest-manifest`) `→ clean` (verified ∅ by the drift
re-check) `| failed`. `clean` is the single in-sync state (drift == ∅), written only off
an actual observation — never a bare apply.

## Repository Operations

### `pudl repo init`

Initialize a `.pudl/` directory in the current repository and install Claude skills into `.claude/skills/`.

```bash
pudl repo init
pudl repo init --force    # Force reinitialize
```

## Interoperability

### `pudl mu`

The `pudl mu` bridge exchanges state with the mu reconciliation plugin.

```bash
pudl mu ingest-observe                        # Ingest observed state from a mu run
mu build --emit-manifest | pudl mu ingest-manifest --model my_model
```

`ingest-manifest` records each applied action and sets the affected resources to
`converging` (applied, pending verification). Passing `--model <name>` tags those rows
with the model so a later clean `pudl run <name>` drift re-check promotes exactly that
model's resources from `converging` to `clean`. Without `--model`, promotion falls back
to matching the model's desired-resource names.

| Flag (`ingest-manifest`) | Description |
|------|-------------|
| `--path` | Read manifest JSON from a file (default: stdin) |
| `--origin` | Origin label for catalog entries (default `mu-build`) |
| `--model` | Tag entries with this `#SystemModel` name for exact converging→clean promotion |

### `pudl data search`

Search stored artifacts by definition, method, or other criteria.

```bash
pudl data search
pudl data search --definition prod_instance
pudl data search --definition prod_instance --method list
pudl data search --limit 10
```

### `pudl data latest <definition> <method>`

Show the most recent artifact for a definition/method pair.

```bash
pudl data latest prod_instance list
pudl data latest prod_instance list --raw
```

## Migration

### `pudl migrate identity`

Backfill `resource_id`, `content_hash`, and `version` columns for catalog entries created before identity tracking was added.

```bash
pudl migrate identity
```

## Observations and Facts

### `pudl observe`

Record a structured observation about the codebase. Observations are stored as facts in the bitemporal fact store.

```bash
pudl observe "auth has circular dependency with user" --kind obstacle --scope pudl:pkg/auth
pudl observe "all db calls use single connection pool" --kind pattern
pudl observe "Config struct has 47 fields" --kind suggestion --scope pudl:internal/config --source claude-code
```

| Flag | Default | Description |
|------|---------|-------------|
| `--kind` | `fact` | Observation kind: fact, obstacle, pattern, antipattern, suggestion, bug, opportunity |
| `--scope` | (none) | Scope as repo:path (e.g. `pudl:internal/database`) |
| `--source` | OS username | Who made the observation |

### `pudl facts list`

Query facts from the bitemporal store.

```bash
pudl facts list --relation observation
pudl facts list --relation observation --source claude-code
pudl facts list --relation depends --as-of-valid 2026-04-01T14:30:00Z
pudl facts list --relation observation -v
pudl facts list --relation observation --json
```

| Flag | Description |
|------|-------------|
| `--relation` | Relation to query (required) |
| `--source` | Filter by source |
| `--as-of-valid` | Query at a point in valid time (RFC3339 or Unix timestamp) |
| `--as-of-tx` | Query at a point in transaction time |
| `-v, --verbose` | Show full fact details |

### `pudl facts show`

Inspect a single fact by ID or unique prefix.

```bash
pudl facts show c0b4392d347a
pudl facts show c0b4392d347a --json
```

### `pudl facts retract`

Mark a fact as retracted ("we were wrong"). Sets `tx_end` -- the fact disappears from current queries but remains in the audit trail.

```bash
pudl facts retract c0b4392d347a
```

### `pudl facts invalidate`

Mark a fact as no longer valid ("reality changed"). Sets `valid_end` -- the fact disappears from current queries but remains visible in historical queries via `--as-of-valid`.

```bash
pudl facts invalidate c0b4392d347a
```

### `pudl facts stats`

Aggregate statistics over the fact store. Groups facts by relation, kind, scope, source, or any arg field.

```bash
pudl facts stats                                        # count by relation
pudl facts stats --relation observation                 # count by kind (default for single relation)
pudl facts stats --relation observation --group-by kind # explicit grouping
pudl facts stats --group-by scope                       # count per scope
pudl facts stats --group-by kind,scope                  # cross-tabulation
pudl facts stats --relation observation --group-by source
```

| Flag | Description |
|------|-------------|
| `--relation` | Filter to specific relation |
| `--group-by` | Comma-separated arg fields to group by (e.g. `kind,scope`) |

Without `--group-by`: defaults to grouping by `relation` (or `kind` if `--relation` is set).

### `pudl pull`

Retrieve all facts related to a scope or entity. Supports prefix matching on scope, plus filtering by kind, source, and relation.

```bash
pudl pull procyon-park:src/cli          # all facts scoped here
pudl pull procyon-park                  # all facts in this repo (prefix match)
pudl pull --kind bug                    # all bugs across all scopes
pudl pull --source claude-code          # everything from this source
pudl pull procyon-park --kind bug       # bugs in procyon-park
pudl pull maggie:vm --json              # machine-readable
```

| Flag | Description |
|------|-------------|
| `--kind` | Filter by observation kind (bug, obstacle, pattern, etc.) |
| `--source` | Filter by source |
| `--relation` | Filter by relation (default: all) |

Output is grouped by scope, showing description, kind, source, and date.

## Datalog and Rules

### `pudl query`

Evaluate Datalog rules over the fact store and catalog, then query results. Rules are loaded from `.pudl/schema/pudl/rules/` (repo-scoped) and `~/.pudl/schema/pudl/rules/` (global).

```bash
pudl query depends_transitive
pudl query depends_transitive from=api
pudl query observation kind=obstacle
pudl query at_risk -f my-analysis.cue
pudl query depends_transitive --json
```

| Flag | Description |
|------|-------------|
| `-f, --rule-file` | Load additional rules from a CUE file |
| `--all-workspaces` | Include global rules and all workspace data |

### `pudl rule add`

Validate and install a Datalog rule file. The file must contain valid CUE with at least one `#Rule`-shaped value (head + body fields).

```bash
pudl rule add transitive-deps.cue           # repo-scoped
pudl rule add company-standards.cue --global # global
```

| Flag | Description |
|------|-------------|
| `--global` | Install to `~/.pudl/schema/pudl/rules/` instead of `.pudl/schema/pudl/rules/` |

## CUE Module Management

### `pudl module`

Manage CUE module dependencies for the schema repository.

```bash
pudl module tidy              # Fetch and update module dependencies
pudl module list              # List current module dependencies
pudl module info              # Show module information
pudl module add <module@ver>  # Add a third-party module dependency
```
