# PUDL -- Personal Unified Data Lake

PUDL is a CLI tool for building a local, schema-validated data lake. Import JSON, YAML, CSV, and NDJSON from any source -- PUDL detects formats, infers CUE schemas, deduplicates by content hash, tracks provenance, and detects drift. Everything runs locally: one binary, a SQLite catalog, and CUE schemas in a git-tracked directory.

## Quick Start

Install the current development version with Go 1.26.2 or newer and put Go's
binary directory (normally `~/go/bin`) on your `PATH`:

```bash
go install github.com/chazu/pudl@main
```

Then follow the [Git inventory walkthrough](docs/getting-started.md). Create a
new Git repository and install its bundled model and data using PUDL commands:

```bash
pudl init
pudl example install git-inventory
```

The guide imports a clean baseline and a changed observation, finds `release`
versus expected `main`, and inspects retained evidence. It needs only PUDL and
Git, with no source checkout, scripts, external accounts, mu, or Python.

## What Happens When You Import

```
pudl import --path data.json
    |
    +-- SHA256 content hash -> deduplicate (skip if already imported)
    +-- Detect format (json, yaml, csv, ndjson)
    +-- Detect NDJSON collections and unwrap typed envelopes when present
    +-- Infer schema via heuristics + CUE unification
    +-- Extract resource identity (stable ID across re-imports)
    +-- Store raw file in .pudl/data/raw/YYYY/MM/DD/ (repo workspace)
    |                   or ~/.pudl/data/raw/YYYY/MM/DD/ (global mode)
    +-- Catalog in SQLite with full provenance metadata
```

Data is never rejected -- if no specific schema matches, it falls back to the universal `pudl/core.#Item` catchall.

## Key Concepts

- **Content-addressed IDs**: Files are identified by SHA256 hash, displayed as pronounceable [proquint](https://arxiv.org/html/0901.4016) words like `mivof-duhij`
- **CUE schemas with `_pudl` metadata**: Schemas define both data shape and inference hints (identity fields, tracked fields)
- **Schema inference**: Heuristic scoring narrows candidates, then CUE unification validates matches -- most specific schema wins
- **Resource identity**: Same logical resource tracked across re-imports via `resource_id` (schema + identity fields hash)
- **Collections**: NDJSON files are split into individual items; normalized memberships allow one item to appear in multiple collections
- **Envelopes**: Typed `{schema, definitions?, data}` JSON records preserve schema metadata while importing the inner payload
- **System models**: A `#SystemModel` instance declares the desired state of a system as a set of desired resources; `pudl run` drives it
- **Value wiring**: required scalar `inputs` bind to explicitly authorized
  `@pudl(binding=plain)` fields from successful producer snapshots; sealed values
  stay inside mu's provider path
- **Exact run-sets**: `pudl run set <models...>` orders only the named models,
  pins producer observations, and never expands the set implicitly
- **Repository isolation**: `pudl init` creates a self-contained `.pudl/`;
  its schemas, imports, catalog, facts, reports, snapshots, and approvals do not
  mutate the global `~/.pudl/` state
- **Drift detection**: A phase of `pudl run` -- compare declared desired state against observed/imported data using deep diff
- **Bitemporal fact store**: General-purpose store for typed assertions (observations, dependencies, derived facts) with full valid-time and transaction-time tracking
- **mu bridge**: pudl declares desired state and renders it to sources; the [mu](https://github.com/...) build tool executes and reconciles. pudl has no execution layer.

See [docs/concepts.md](docs/concepts.md) for a deeper explanation of these ideas.

## Commands

### Data Import and Catalog

| Command | Description |
|---------|-------------|
| `pudl init` | Initialize or repair local `.pudl/` (`--global` selects `~/.pudl/`) |
| `pudl import --path <file>` | Import data with automatic detection |
| `pudl list` | Query catalog (filter by `--schema`, `--origin`, `--format`, etc.) |
| `pudl show <id>` | Inspect an entry (`--raw`, `--metadata`) |
| `pudl delete <id>` | Remove entry from catalog |
| `pudl export` | Export data by ID, schema, or origin to JSON/YAML/CSV/NDJSON |

### Schema Management

| Command | Description |
|---------|-------------|
| `pudl schema list` | List schemas (`--package`, `--verbose`) |
| `pudl schema add <name> <file>` | Add a schema to the repository |
| `pudl schema new --from <id>` | Generate CUE schema from imported data |
| `pudl schema show <name>` | Display schema details |
| `pudl schema migrate` | Run schema migrations |
| `pudl schema reinfer` | Re-infer schemas for existing entries |

### Models

| Command | Description |
|---------|-------------|
| `pudl model list` | List registered `#SystemModel` instances with last-run status |
| `pudl model show <name>` | Show a model's desired entries and details |
| `pudl model validate <name>` | Validate an authored model template; bound values are concretely revalidated at run time |
| `pudl run <name>` | Observe-only ACUTE loop: populate -> drift -> checks -> report |
| `pudl run <name> --converge` | Close drift: pudl renders desired->sources, the mu plugin reconciles |
| `pudl run set <models...>` | Observe an exact producer/consumer set in dependency order |
| `pudl run set <models...> --converge` | Preflight and plan the whole exact set, then mutate; sealed-output sets pause for mandatory exact-plan approval |
| `pudl run report [id]` | Read the latest or named standalone/set report |
| `pudl run resume/reject <id>` | Approve or reject a pending standalone/set operation |
| `pudl status` | Read catalog convergence status recorded by the last model run |

### Facts

| Command | Description |
|---------|-------------|
| `pudl facts add --relation NAME --args JSON` | Record a generic assertion (`--source`, optional `--schema`) |
| `pudl facts list --relation <name>` | Query facts by relation with temporal filtering (`--as-of-valid`, `--as-of-tx`) |
| `pudl facts show <id>` | Inspect a single fact (supports ID prefix matching) |
| `pudl facts retract <id>` | Mark a fact as retracted (assertion was wrong) |
| `pudl facts invalidate <id>` | Mark a fact as no longer valid (reality changed) |

See [docs/facts.md](docs/facts.md) for the bitemporal fact store documentation.

### Datalog and Rules

| Command | Description |
|---------|-------------|
| `pudl query <relation> [key=value ...]` | Evaluate rules and query derived facts |
| `pudl rule add <file>` | Validate and install a Datalog rule file (`--global`) |

See [docs/datalog.md](docs/datalog.md) for the evaluator documentation and rule authoring guide.

### Workspace Operations

| Command | Description |
|---------|-------------|
| `pudl doctor` | Workspace health, catalog validation, and inference stability |
| `pudl example install git-inventory` | Install the bundled Git inventory model and sample observations |
| `pudl config` | Show current configuration |

See [docs/cli-reference.md](docs/cli-reference.md) for the full command reference.

## Command consolidation

The overlapping command paths have been consolidated. Update existing scripts:

| Former command | Current command |
| --- | --- |
| `pudl repo init` | `pudl init` |
| `pudl init` for global state | `pudl init --global` |
| `pudl catalog` | `pudl schema list` (`--verbose` includes metadata details) |
| `pudl model describe NAME` | `pudl model show NAME` (`--discover` adds Mu capabilities) |
| `pudl validate --all` or `pudl verify` | `pudl doctor` |
| `pudl validate --entry ID` | `pudl doctor --entry ID` |
| `pudl run-set MODELS...` | `pudl run set MODELS...` |
| `pudl run-set report/resume/reject ID` | `pudl run report/resume/reject ID` |
| `pudl list --all-workspaces` | `pudl list` (active catalog, explicit `--origin` filter) |

Former paths are removed rather than retained as aliases. Schema/Git and
module/CUE helper commands remain available. Standalone runs and exact sets
retain their existing evidence-selection and approval semantics; shared report
and approval commands route by the stored operation ID.

## Agent memory removal

The agent self-improvement application has been removed: `memory`, `hooks`,
`pull`, `facts observe/promote/curate`, automatic observation/feedback schemas,
and the `fact_scored` decay relation. `reflect_command` is no longer used.

Generic `facts add/list/show/search/stats/retract/invalidate`, temporal history,
transactions, and Datalog remain available. Stored facts are preserved.
Migration 18 drops only the old derived scoring view on the next writable
catalog open. Workspace initialization retires the unmodified shipped nous
schema while preserving authored replacements and symlinks.

For existing integrations, remove hooks invoking `pudl memory context`,
`pudl facts curate`, or `pudl hooks suggest`, and retire generated Mu
`//memory:*` targets. Customized workflows are user-owned; PUDL no longer
installs or runs them.

## Writing Custom Schemas

PUDL schemas are CUE files with embedded `_pudl` metadata that drives inference:

```cue
package ec2

#Instance: {
    _pudl: {
        schema_type:     "base"
        resource_type:   "aws.ec2.instance"
        identity_fields: ["InstanceId"]
        tracked_fields:  ["State", "InstanceType", "Tags"]
    }
    InstanceId:   string
    InstanceType: string
    State:        { Name: string }
    ...
}
```

See [docs/schema-authoring.md](docs/schema-authoring.md) for the full guide.

## The mu Integration

PUDL knows what has drifted but does not execute changes itself. When you run a model with `--converge`, PUDL renders the desired state to sources; the mu build tool's plugin then reconciles reality to match. This separation keeps PUDL focused on data and schema correctness while mu handles execution.

```bash
# Observe-only: populate, detect drift, run checks, report
pudl run my-server

# Close drift: pudl renders desired -> sources, mu reconciles
pudl run my-server --converge

# Coordinate cross-model values without implicit producer discovery
pudl run set network my-server
```

Plain values are persisted with source/snapshot provenance. Sealed inputs and
outputs stay in mu's provider path, while PUDL stores only schemes and
fingerprints. Generated targets use strict action routing: unused declarations,
undeclared claims, and ambiguous output writers fail during whole-set planning,
before mutation or provider traffic. A set that can write a sealed output always
pauses for exact-plan approval; resume rebuilds and revalidates that plan, then
mu compares the same-workspace raw digest and executes that exact in-memory
graph before producer-first execution.

## Documentation

See [docs/README.md](docs/README.md) for the full documentation index.

## Project Status

PUDL's data pipeline (import, catalog, schema inference, drift detection, mu bridge) is stable. The old execution runtime (Glojure-based methods and workflows) was removed in a major refactoring to focus PUDL on what it does best: ingesting data, inferring schemas, and detecting drift. Execution is delegated to mu; PUDL declares desired/observed state and converges it through `#SystemModel` instances run with `pudl run`.

See [docs/VISION.md](docs/VISION.md) for the roadmap.

## Requirements

- Go 1.26.2+
- Git (for schema version control)
- CUE ([cuelang.org](https://cuelang.org)) for schema definitions
