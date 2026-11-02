---
name: pudl-core
description: Using the pudl CLI — data lake import/query, CUE schemas, the bitemporal fact store, and #SystemModel checks that observe with commands, compare with desired state and evaluate checks. Use when working with pudl commands, schemas, facts, or checking system models.
---

# PUDL Core

CLI for an infrastructure data lake: import data, infer/validate CUE schemas,
store bitemporal facts, and check `#SystemModel` instances that observe real
infrastructure with plain commands, compare it with desired state and evaluate
checks.

PUDL discovers, catalogs and governs; it never changes the systems it observes.
A check reports drift; Terraform, kubectl or whatever tool owns the system
applies changes.

## Agent routing

Use the binary as the source of truth for the current command surface:

- `pudl help --json` — complete command tree, flags, and descriptions.
- `pudl guide <topic>` — operational guidance (`overview`, `models`,
  `troubleshooting`, and the other listed topics).
- `pudl model show <name> --json` — a model's actual runtime contract.

Scaffold first, then edit the returned path:

- `pudl model new <name> --populate 'command:<cmdline>'` — a command printing JSON records, run by pudl (without `--populate`, a placeholder command to replace)
- `pudl rule new <name>`

For a one-off observer, use `pudl check --populate 'command:<cmdline>'`; it
writes no model definition. Retrieve diagnostics with
`pudl report [<id>] --json`.

## State Layout

```
.pudl/                   # active inside a repo; ~/.pudl/ outside one
  workspace.cue          # repo identity and policy (repository mode)
  config.yaml            # local path configuration
  schema/                # CUE module; repo copy is tracked by enclosing Git
    cue.mod/             # CUE module metadata
    pudl/                # built-in + local schema defs (incl. #SystemModel)
    pudl/rules/          # Datalog rules
  populators/            # helper programs and fixtures models' commands use
  data/
    raw/                 # content-addressed imports
    metadata/            # provenance sidecars
    sqlite/catalog.db    # catalog, facts, reports, snapshots
```

`pudl init` is idempotent and repairs this local layout. Repository and
global catalogs are independent; mutable state never falls back across them.

## Common Commands

### Data pipeline
- `pudl import <file|dir|glob>...` — import JSON/YAML/CSV/NDJSON (schema inferred unless `--schema` given; `--path` also works, `-` for stdin)
  - `--dry-run` first: per-schema counts, validation failures, unresolved identity, facts that would be projected; writes nothing
  - `--set path=value` (repeatable, overwrites) adds a field the source omits, e.g. `--set project=prod-a` for gcloud output
  - re-importing cataloged data with a `--schema` it satisfies moves it to that schema
- `pudl list` — list entries in the active catalog
  (`--origin` filters explicitly; `--artifacts` = run outputs; `--schema` with `#` matches whole definition names; `--json` is paginated, count with `total_matched`)
- `pudl show <id>` / `pudl export --id <proquint> --format json` (exact payload, pipeable) / `pudl delete <id>`
  (`show` includes a compliance section: each applicable policy's status and violations)
- `pudl policy list [--type T]` / `pudl policy show POLICY [--limit N]` — compliance policies, result counts (stale ones included), rules and the resources failing most; `pudl policy evaluate` refreshes results
- `pudl doctor` — workspace health, assigned-schema validation, and inference stability

### Schema
- `pudl schema list|show <name>` — browse schemas
- `pudl schema new --from <id> --path <package>/#<Definition>` — generate a schema from data
- `pudl schema add` — register a definition (e.g. a `#SystemModel`)
- `cue mod get <module@version>` — add CUE module deps

### Facts
- `pudl facts` — query the bitemporal fact store
- `pudl query <relation> [key=value ...]` — derived facts via Datalog rules
  (positional `key=value` constraints, not `--where`); `pudl rule` manages rules
- `pudl query --list` — list queryable relations (rule heads + EDB facts) and their arg keys
- `pudl query --topo <relation>` — read a relation's `from`/`to` edges as a topological order (errors on a cycle)
- `pudl doctor --fix [--dry-run] [--only NAME]` — apply catalog repairs: schema-names, policy-leaves, inference, identity, projections, policy-results (projected-fact sync also runs automatically after import/check/delete)

### Querying imported data

Imported payloads are not visible to Datalog until a schema projects them.
Declare relations in `_pudl.facts`, then write rules and checks over them —
don't post-process payloads with jq:

```cue
_pudl: {
	identity_fields: ["project", "name"]          // required for facts; quote dotted keys: "a.\"b.c\""
	facts: {
		gcp_firewall: args: {project: "project", name: "name", disabled: {path: "disabled", default: false}}
		gcp_firewall_allow: {each: "allowed[*]", args: {proto: "IPProtocol", port: {path: "ports[*]", default: "*"}}}
	}
	sensitive_fields: ["env[*].value"]           // stored as "[REDACTED]"; fails closed
}
```

- Every projected fact carries `entry_id` and `resource_id`; join relations of one record on `entry_id`.
- Facts follow each resource's most recently observed record.
- `{exists: path}` and `default` stand in for negation (the Datalog has none).
- CLI constraints are typed: `disabled=false`, `port=22`; `name='"22"'` forces a string.
- `pudl query` and model checks warn on stderr about stale projection and about rules referencing relations/args nothing produces — read those warnings when a check passes unexpectedly.
- Route sensitive data with `--schema` (or a model's `populate.schema`).
- Ownership: `_pudl: owner: {paths: ["labels.team"], map: {path: "project", owners: _projectOwners}, fallback?: "team"}`
  (tried in that order, inherited along `base_schema`) projects `resource_owner(resource_id, owner, via, type)` or
  `resource_unowned(resource_id, type)`. Check findings carry `owner`; `pudl check --owner TEAM` and
  `pudl policy show P --owner TEAM` filter to one owner; a check on `resource_unowned` lists unowned resources.

See `docs/projection.md`.

### Compliance policies

Single-resource rules are policies, not Datalog rules: a standalone definition
with `_pudl: {schema_type: "policy", base_schema: "<pkg>.#Type"}` (never
`#Type & {...}`, which does not load). It is evaluated at every ingest against
each resource of that type; results are `policy_status`/`policy_violation`
facts. Checks query the built-in `policy_noncompliant(resource, policy,
severity)`.

- `field: value @pudl(rule="name", severity="low|medium|high|critical", doc="...")`; an absent required field makes the result `unknown` unless `@pudl(absent="violation")` is on it or an ancestor, or the type has `_pudl.review: {status: "validated"}`.
- Annotate fields outside comprehensions (`f?: _ @pudl(rule=...)`), else the policy fails to load.
- `&&`/`||` do not short-circuit over absent fields; treat absent as a value with `list.Contains([if x != _|_ {x}], v)`.
- A field required only inside a comprehension is never reported absent; anchor the rule as a conflict on a field that is present.
- `pudl policy evaluate --what-if draft.cue` tries a draft without writing; `--dry-run` previews a refresh.

See `docs/schema-authoring.md#compliance-policies`.

### #SystemModel loop
- `pudl model list` — list registered `#SystemModel` definitions + last status
- `pudl model show <model>` — show a model's populate/desired/checks
- `pudl model validate <model>` — structural validation without running
- `pudl check <model>...` — check exactly the named models, producers first by `depends_on`: populate, drift, checks; one report for all of them; never runs a model that was not named
- `pudl check <model>... --detailed-exitcode` — exit 0 clean, 2 drift or failing check, 1 error
- `pudl check <model>... --fail-on new [--detailed-exitcode]` — fail (or exit 2) on fail-severity check failures only when one has a finding no comparable earlier report recorded; drift and errors still count
- `pudl check <model> --from-catalog --catalog-scope <snapshot-or-origin>` — explicitly replay ingested records for inventory drift (one model); a normal inventory check populates and compares its own current snapshot
- `pudl check <model>... --check-upstream` — warn if any transitive upstream (depends_on) model is `drifted`/`failed`
- `pudl report [id]` — read a check report (or one model's report by run ID); stored `run`/`run set` reports read too, with retired fields shown as legacy JSON
- `pudl report [id] --since <id2> [--json]` — classify each check's failing matches against an earlier report: new, persisting, resolved, unknown, with first-seen times
- `pudl model deps` — reconcile + show the cross-model dependency graph (no check needed)
- `pudl status [target]` — recorded verdict by catalog target (a check records each model's verdict)

### Utilities
- `pudl init` / `pudl init --global` / `pudl doctor` / `pudl config` / `pudl version`
- `pudl guide` / `pudl prime` — agent-facing usage reference

## The #SystemModel loop

`pudl check <model>...` resolves each named `#SystemModel` definition (project
`.pudl/schema` precedes explicitly vendored dependencies; register with
`pudl schema add`) and runs each, producers first:

```
1. populate -> ingest   (Accumulate observed state into the catalog)
2. drift                (Unify desired vs observed)
3. checks               (flag violations)
4. report
```

- **Populate arm**: `#CommandObserve` — `runs: [{argv: [...], set?: {...}}]`
  plus optional `schema:` — whose commands pudl runs itself (no shell; fan out
  with a CUE comprehension; the environment is inherited, so cloud CLI
  credentials work). A model without `populate` is checks-only: it evaluates
  its checks over the catalog as it stands (e.g. imported data).
- **Failed inputs**: a failing run does not discard the others. Their records
  are ingested, the snapshot is incomplete, the report lists the failed run
  under `populate.failed_inputs` (argv, exit status, stderr tail), checks over
  it are unknown, and the model fails. `fail_fast: true` restores
  all-or-nothing.
- **Enumerated inputs**: `each: {argv: [...], select?: "field"}` runs a command
  that prints a JSON list (strings, or objects with `select` naming the field);
  every run is repeated per item with `"{{item}}"` replaced in its argv and
  `set`. The report records the items under `populate.inputs`.
- **Drift** set-diffs `desired` against the records the check just observed and
  reports `missing`/`changed`/`unidentifiable`/`ambiguous` findings. Nothing is
  changed.
- **Waivers**: `waivers: [...]` on the model accepts findings until a date.
  Each needs `owner`, `reason`, `created`, `expires` (`YYYY-MM-DD`) and a
  target: `check` + `witness: {arg: value}` (an `expect: "empty"` check's
  witnesses), or `policy` + `rule` + `resource` (a policy violation behind a
  check whose witnesses carry `resource` and `policy`). A fully waived check
  has outcome `waived`, stays in the report, and does not fail the check or
  `--detailed-exitcode`; expired waivers stop applying and are flagged; waivers
  matching nothing are reported `stale`.
- **Retired fields** — `converge`, `plugins`, `sealed_inputs`,
  `sealed_outputs`, `inputs`, `bindings`, `#PluginObserve`, `#EweTarget` — fail
  to load with an error naming the field (`docs/retired-commands.md`).
- **One report**: `pudl check --json` writes one report (`report_version` 3)
  with an aggregate `summary` and one entry per model under `members`, each
  embedding that model's full report (`members[].report`).
- Each check records the model instance in the catalog (identity = name) so it's
  inventoriable via `pudl list` / `pudl query`.

### Workspace schema precedence

Repository definition resolution uses local schemas and explicitly declared
packages under `.pudl/vendor/`. `workspace.cue` lists their relative roots in
`dependencies`. Ambient global definitions are excluded. Use `pudl --global`
for personal state, `pudl config --paths --json` for effective paths, and
`pudl config --legacy-dependencies --json` to inventory migration candidates.

## Cross-model dependencies

A model can depend on another model's output. Declare it with `depends_on` (a
list of model **names**) on the `#SystemModel`:

```cue
#Workloads: sm.#SystemModel & { name: "workloads", depends_on: ["network"], ... }
```

`pudl check` (and `pudl model deps`) reconcile declared deps into bitemporal
`model_depends_on(from,to)` facts. Built-in recursive Datalog rules reason over
them (query with positional `key=value`):

- `pudl query depends_transitive from=<m>` — what `<m>` depends on (transitively)
- `pudl query impacted_by changed=<m>` — blast radius: who depends on `<m>`
- `pudl query cyclic` — models in a dependency cycle (no valid run order)
- `pudl query --topo model_depends_on` — a topological run order (deps first)

`pudl model deps` records edges for **every** registered model without running
them. Dependencies come from explicit `depends_on` declarations. PUDL does not
guess edges from coincidental values or check downstream models automatically.
`pudl check a b` orders `a` and `b` by the `depends_on` edges between them,
rejects cycles before anything runs, and records a model whose named producer
did not succeed as `blocked`. See `docs/cross-model-dependencies.md`.

## Evidence workflow

Use `pudl example install gcp-network-hygiene` for a credential-free fixture.
Its live gcloud model is separately named. The tutorial is
`docs/gcp-network-hygiene.md`. `list --where path=value --select path --all`
inspects payloads before pagination. `show --field` reads one field; `show
--history` lists resource versions and sightings; `snapshot show NEW --compare
OLD` compares matching populations. `list --fancy` is retained.

Save a query as a check with `model new NAME --check RELATION --evidence
scope:POPULATION --max-age 15m`. Check outcomes are pass/fail/waived/unknown/error.
Missing, broken, incomplete or stale required evidence cannot pass. Reports
include separate execution/conformity/checks/verification summaries and retained
evidence; age_at_check is frozen. Each check result records `findings` (every
failing match by a stable key: head arguments without evidence IDs) with a
state — new, persisting, resolved, unknown — and `first_seen`, and `changes`
against the model's comparable earlier reports. Only a complete observation
resolves; a partial or failed one leaves earlier findings unknown. Waived
findings keep their state, are marked `waived`, and never count for
`--fail-on new`. `doctor --json` lists repair diagnoses with
preview/apply arguments; only `doctor --fix` writes. Explicit --schema is strict; exploratory
fallback requires --allow-schema-fallback.
