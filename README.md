# PUDL -- Personal Unified Data Lake

PUDL is a CLI tool for building a local, schema-validated data lake. Import JSON, YAML, CSV, and NDJSON from any source -- PUDL detects formats, infers CUE schemas, deduplicates by content hash, tracks provenance, and detects drift. Everything runs locally: one binary, a SQLite catalog, and CUE schemas in a git-tracked directory.

## Quick Start

Install the current development version with Go 1.26.10 or newer and put Go's
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

The guide observes a clean baseline and a changed inventory, finds `release`
versus expected `main`, and inspects retained evidence. It needs only PUDL and
Git, with no source checkout, scripts, external accounts, or Python.

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

Untyped imports fall back to `pudl/core.#Item` when no specific schema matches.
An explicit `--schema` must validate every record; use `--allow-schema-fallback`
only when you deliberately want permissive classification.

## Key Concepts

- **Content-addressed IDs**: Files are identified by SHA256 hash, displayed as pronounceable [proquint](https://arxiv.org/html/0901.4016) words like `mivof-duhij`
- **CUE schemas with `_pudl` metadata**: Schemas define both data shape and inference hints (identity fields, tracked fields)
- **Schema inference**: Heuristic scoring narrows candidates, then CUE unification validates matches -- most specific schema wins
- **Resource identity**: Same logical resource tracked across re-imports via `resource_id` (schema + identity fields hash)
- **Collections**: NDJSON files are split into individual items; normalized memberships allow one item to appear in multiple collections
- **System models**: A `#SystemModel` instance declares how to observe a system (commands that print JSON records), its desired state and its checks; `pudl check` drives it
- **One check, one report**: `pudl check <models...>` runs only the named
  models, producers first by `depends_on`, never expands the set implicitly,
  and records one report for one model or many
- **Repository isolation**: `pudl init` creates a self-contained `.pudl/`;
  its schemas, imports, catalog, facts, reports, and snapshots do not
  mutate the global `~/.pudl/` state
- **Drift detection**: A phase of `pudl check` -- compare declared desired state against observed/imported data using deep diff. PUDL reports drift and changes nothing
- **Bitemporal fact store**: General-purpose store for typed assertions (observations, dependencies, derived facts) with full valid-time and transaction-time tracking

See [docs/concepts.md](docs/concepts.md) for a deeper explanation of these ideas.
See [evidence and recovery](docs/evidence.md) for exact observations, faithful export,
classification explanations, historical report values, bounded check witnesses,
independent evidence retention, and verified portable workspace bundles.

## Commands

### Data Import and Catalog

| Command | Description |
|---------|-------------|
| `pudl init` | Initialize or repair local `.pudl/` (`--global` selects `~/.pudl/`) |
| `pudl import --path <file\|dir\|pattern>` | Import data with automatic detection (stdin when piped; `--recursive` for directories) |
| `pudl list` | Query catalog (filter by `--schema`, `--origin`, `--format`, etc.) |
| `pudl show <id>` | Inspect an entry (`--raw`, `--metadata`) |
| `pudl delete <id>` | Remove entry from catalog |
| `pudl export` | Export data by ID, schema, or origin to JSON/YAML/CSV/NDJSON |
| `pudl snapshot list/show/current` | Inspect retained observation snapshots |
| `pudl snapshot retain/prune` | Pin snapshots, or remove ones a retention policy no longer wants |

### Schema Management

| Command | Description |
|---------|-------------|
| `pudl schema list` | List schemas (`--package`, `--verbose`) |
| `pudl schema add <package>.<name> <cue-file>` | Add a schema to the repository |
| `pudl schema new --from <id>` | Generate CUE schema from imported data |
| `pudl schema show <schema-name>` | Display schema details |

### Models

| Command | Description |
|---------|-------------|
| `pudl model list` | List registered `#SystemModel` instances with last status |
| `pudl model show <name>` | Show a model's desired entries and details |
| `pudl model validate <name>` | Validate an authored model without running it |
| `pudl model new <name>` | Scaffold a `#SystemModel` observer (`--populate 'command:<cmdline>'`) |
| `pudl model deps` | Refresh and show the cross-model dependency graph |
| `pudl check <models...>` | Observe-only loop for exactly the named models, producers first: populate -> drift -> checks -> one report |
| `pudl report [id]` | Read the latest or named check report (also stored `run`/`run set` reports) |
| `pudl status` | Read the verdict recorded by each model's last check |

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
| `pudl rule new <name>` | Scaffold a Datalog rule |

See [docs/datalog.md](docs/datalog.md) for the evaluator documentation and rule authoring guide.

### Workspace Operations

| Command | Description |
|---------|-------------|
| `pudl doctor` | Workspace health, catalog validation, and inference stability; `--fix` applies catalog repairs |
| `pudl example install git-inventory` | Install the bundled Git inventory model and sample observations |
| `pudl config` | Show current configuration |
| `pudl guide [topic]` / `pudl prime` | Quick-reference guide and agent prompt |

See [docs/cli-reference.md](docs/cli-reference.md) for the full command reference (generated
from the command tree; `pudl help --json` prints it in machine-readable form).

## Command consolidation

The overlapping command paths have been consolidated. Update existing scripts:

| Former command | Current command |
| --- | --- |
| `pudl repo init` | `pudl init` |
| `pudl init` for global state | `pudl init --global` |
| `pudl catalog` | `pudl schema list` (`--verbose` includes metadata details) |
| `pudl model describe NAME` | `pudl model show NAME` |
| `pudl validate --all` or `pudl verify` | `pudl doctor` |
| `pudl validate --entry ID` | `pudl doctor --entry ID` |
| `pudl run MODEL`, `pudl run set MODELS...`, `pudl run-set MODELS...` | `pudl check MODELS...` |
| `pudl run report ID`, `pudl run-set report ID` | `pudl report ID` |
| `pudl run --converge`, `run resume/reject`, `pudl mu ...` | Retired with convergence and mu ([retired commands](docs/retired-commands.md#convergence-and-mu-integration)) |
| `pudl list --all-workspaces` | `pudl list` (active catalog, explicit `--origin` filter) |
| `pudl schema migrate`, `schema reinfer`, `migrate identity`, `reclassify`, `facts reproject` | `pudl doctor --fix` ([repairs](docs/retired-commands.md#catalog-repairs)) |

Former paths are removed rather than retained as aliases. Git/editor/CUE wrappers
are also retired; see [native replacements](docs/retired-commands.md). Every check, and every
stored run or run-set report, is read with `pudl report`, which routes by the stored ID.

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

## Observing systems

PUDL discovers, catalogs and governs infrastructure; it does not change it. A
model observes by running plain commands that print JSON records, compares what
it saw with its desired state, and evaluates checks. Terraform, kubectl or
whatever tool owns a system applies the changes; the next run verifies them.

```cue
#Firewalls: sm.#SystemModel & {
	name: "firewalls"
	populate: {
		schema: "pudl/gcp.#Firewall"
		runs: [{argv: ["gcloud", "compute", "firewall-rules", "list", "--format=json"]}]
	}
}
```

```bash
# Populate, detect drift, run checks, report
pudl check firewalls --detailed-exitcode

# Check several models, producers first by depends_on; one report
pudl check network my-server
pudl report
```

## Documentation

See [docs/README.md](docs/README.md) for the full documentation index.

## Project Status

PUDL's data pipeline (import, catalog, schema inference, observation, drift detection, checks) is stable. The old execution runtime and, later, the convergence workflow and mu integration were removed to focus PUDL on what it does best: observing systems, cataloging what it sees, and reporting drift and check results. `#SystemModel` instances checked with `pudl check` declare what to observe and what to expect.

See [docs/VISION.md](docs/VISION.md) for the roadmap.

## Requirements

- Go 1.26.10+
- Git (for schema version control)
- CUE ([cuelang.org](https://cuelang.org)) for schema definitions
