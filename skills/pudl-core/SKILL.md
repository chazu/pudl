---
name: pudl-core
description: Using the pudl CLI — data lake import/query, CUE schemas, the bitemporal fact store, and the #SystemModel observe/converge loop that drives mu. Use when working with pudl commands, schemas, facts, or running/converging system models.
---

# PUDL Core

CLI for an infrastructure data lake: import data, infer/validate CUE schemas,
store bitemporal facts, and run `#SystemModel` instances that observe and
converge real infrastructure through the **mu** execution layer.

There is no execution layer inside pudl — models, methods and workflows were
extracted to mu. pudl declares desired/observed state; mu mutates the world.

## Agent routing

Use the binary as the source of truth for the current command surface:

- `pudl help --json` — complete command tree, flags, and descriptions.
- `pudl guide <topic>` — operational guidance (`overview`, `models`, `mu`,
  `troubleshooting`, and the other listed topics).
- `pudl model show <name> --json` — a model's actual runtime contract.

Scaffold first, then edit the returned path:

- `pudl model new <name> --populate plugin:<name>`
- `pudl model new <name> --populate 'command:<cmdline>'` — a command printing JSON records, run by pudl (no mu)
- `pudl rule new <name>`
- `pudl model populator new <model>`

For a one-off observer, use `pudl run --populate plugin:<name> --input key=value`
or `pudl run --populate 'command:<cmdline>'`; it writes no model definition and
is observe-only. Retrieve completed or pending
diagnostics with `pudl run report [<run-id>] --json`. Convergence that crosses a
trust boundary can use `--require-approval`, then `pudl run resume` or `reject`.

## State Layout

```
.pudl/                   # active inside a repo; ~/.pudl/ outside one
  workspace.cue          # repo identity and policy (repository mode)
  config.yaml            # local path configuration
  schema/                # CUE module; repo copy is tracked by enclosing Git
    cue.mod/             # CUE module metadata
    pudl/                # built-in + local schema defs (incl. #SystemModel)
    pudl/rules/          # Datalog rules
    populators/          # populator programs for #EweTarget models
  data/
    raw/                 # content-addressed imports
    metadata/            # provenance sidecars
    sqlite/catalog.db    # catalog, facts, reports, snapshots, approvals
```

`pudl init` is idempotent and repairs this local layout. Repository and
global catalogs are independent; mutable state never falls back across them.

## Common Commands

### Data pipeline
- `pudl import <file|dir|glob>...` — import JSON/YAML/CSV/NDJSON (schema inferred unless `--schema` given; typed envelopes preserve schema metadata; `--path` also works, `-` for stdin)
  - `--dry-run` first: per-schema counts, validation failures, unresolved identity, facts that would be projected; writes nothing
  - `--set path=value` (repeatable, overwrites) adds a field the source omits, e.g. `--set project=prod-a` for gcloud output
  - re-importing cataloged data with a `--schema` it satisfies moves it to that schema
- `pudl list` — list entries in the active catalog
  (`--origin` filters explicitly; `--artifacts` = run outputs; `--schema` with `#` matches whole definition names; `--json` is paginated, count with `total_matched`)
- `pudl show <id>` / `pudl export --id <proquint> --format json` (exact payload, pipeable) / `pudl delete <id>`
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
- `pudl facts reproject` — sync facts projected from imported records (runs automatically after import/run/reinfer/delete)

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
- `pudl query` and run checks warn on stderr about stale projection and about rules referencing relations/args nothing produces — read those warnings when a check passes unexpectedly.
- Route sensitive data with `--schema` (or a model's `populate.schema`).

See `docs/projection.md`.

### #SystemModel loop
- `pudl model list` — list registered `#SystemModel` definitions + last-run status
- `pudl model show <model>` — show a model's populate/converge/desired/checks
- `pudl model validate <model>` — structural validation without running
- `pudl run <model>` — run a registered `#SystemModel` (OBSERVE-ONLY by default)
- `pudl run <model> --converge` — close drift (mutates the target via mu)
- `pudl run <model> --from-catalog` — explicitly replay ingested records for inventory drift; a normal inventory run populates and compares its own current snapshot
- `pudl run <model> --check-upstream` — warn if any transitive upstream (depends_on) model is `drifted`/`failed`
- `pudl run set <models...>` — run exactly the named models in producer-first order; no implicit producer expansion
- `pudl run set <models...> --converge` — whole-set read-only preflight and exact planning before mutation; sealed-output sets pause for mandatory approval, while other sets may opt in with `--require-approval`
- `pudl run report|resume|reject` — inspect or decide standalone/set operations
- `pudl model deps` — reconcile + show the cross-model dependency graph (no run needed)
- `pudl model populator add ...` — manage populator programs for `#EweTarget`
- `pudl status [target]` — recorded convergence status by catalog target (a run records its verdict)

### Utilities
- `pudl init` / `pudl init --global` / `pudl doctor` / `pudl config` / `pudl version`
- `pudl guide` / `pudl prime` — agent-facing usage reference

## How pudl drives mu (the #SystemModel loop)

`pudl run <model>` resolves a `#SystemModel` definition (project `.pudl/schema`
precedes explicitly vendored dependencies; register with `pudl schema add`) and runs the
ACUTE cycle:

```
1. populate -> ingest   (Accumulate observed state into the catalog)
2. drift                (Unify desired vs observed)
3. checks               (flag violations)
4. report
```

- **Populate arm**: a plugin (live observe inside an existing mu project,
  discovered via `mu.cue` from the model dir, override with `--mu-root`), an
  `#EweTarget` whose populator self-stages its own temp mu project, or a
  `#CommandObserve` — `runs: [{argv: [...], set?: {...}}]` plus optional
  `schema:` — whose commands pudl runs itself (no mu; fan out with a CUE
  comprehension). A model without `populate` is checks-only: it evaluates its
  checks over the catalog as it stands (e.g. imported data).
- **Default is observe-only** — no mutation. `--converge` opts into the loop:
  `drift==∅ -> clean | iteration cap -> failed | else converge -> execute -> re-observe`; the
  PUDL coordinator owns this lifecycle while mu executes each operation.
  (`--max-iters`, `--dry-run`, `--only <selectors>`). `--only` is a converge-only
  preflight filter: selectors match desired resource names or schema paths and
  include transitive `depends_on` resources; unknown selectors fail before side effects.
- **Converge plugins run hermetically.** mu executes actions with a minimal
  environment (no inherited `HOME`), so a converge plugin that needs host
  credentials must receive them through the model's `converge.input` — e.g. the
  k8s plugin needs `input.kubeconfig: "/path/to/kubeconfig"` or it cannot find
  `~/.kube/config`.
- Each run records the model instance in the catalog (identity = name) so it's
  inventoriable via `pudl list` / `pudl query`.

### mu bridge

mu writes its results back into the pudl schema list via:
- `pudl mu ingest-observe` — ingest observe results (`entry_type=observe`)
- `pudl mu ingest-manifest` — ingest a build manifest (`entry_type=manifest`,
  per-action `manifest-action`); `--model <name>` tags rows so a later clean
  drift re-check promotes the model's `converging` resources to `clean`

`pudl run --converge` renders the model's `desired` state to sources and runs
`mu build --emit-manifest`; the mu plugin reconciles, and pudl ingests the
manifest (per-resource `converging` → `clean` on the re-observe). If the
manifest cannot be persisted, the run is `unknown`/needs verification rather
than clean. pudl computes no provider domain ops.

These `entry_type` values are what `pudl list --artifacts` surfaces (run
outputs), vs ingested/observed data.

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

`pudl run` (and `pudl model deps`) reconcile declared deps into bitemporal
`model_depends_on(from,to)` facts. Built-in recursive Datalog rules reason over
them (query with positional `key=value`):

- `pudl query depends_transitive from=<m>` — what `<m>` depends on (transitively)
- `pudl query impacted_by changed=<m>` — blast radius: who depends on `<m>`
- `pudl query cyclic` — models in a dependency cycle (no valid run order)
- `pudl query --topo model_depends_on` — a topological run order (deps first)

`pudl model deps` records edges for **every** registered model without running
them. Dependencies come from explicit `depends_on` declarations and value
bindings. PUDL does not guess edges from coincidental values or re-run downstream
models automatically. See `docs/cross-model-dependencies.md`.

### Value bindings

For actual value flow, a model template declares required scalar `inputs` and
matching `bindings`. Both the consumer slot and source schema field must carry
`@pudl(binding=plain)`. A standalone run may reuse the latest successful scoped
producer snapshot but never starts it. `pudl run set` names the exact closed set,
rejects missing producers/cycles before execution, and pins current-run producer
observations for downstream resolution.

Sealed bindings never pass through the catalog. Mu's provider channel resolves
and stores them at execution time; PUDL records only schemes and fingerprints.
Generated targets require strict per-action claims. Unused declarations,
undeclared claims, and ambiguous output writers fail during whole-set planning
before mutation or provider traffic. A run-set that can write a sealed output
always pauses for exact-plan approval, and resume rebuilds and revalidates that
plan. Each apply passes mu the raw same-workspace plan digest; mu compares it
before provider access and executes the same in-memory graph.

## Evidence workflow

Use `pudl example install gcp-network-hygiene` for a credential-free fixture.
Its live gcloud model is separately named. The tutorial is
`docs/gcp-network-hygiene.md`. `list --where path=value --select path --all`
inspects payloads before pagination. `show --field` reads one field; `show
--history` lists resource versions and sightings; `snapshot show NEW --compare
OLD` compares matching populations. `list --fancy` is retained.

Save a query as a check with `model new NAME --check RELATION --evidence
scope:POPULATION --max-age 15m`. Check outcomes are pass/fail/unknown/error.
Missing, broken, incomplete or stale required evidence cannot pass. Reports
include separate execution/conformity/checks/verification summaries and retained
evidence; age_at_check is frozen. Use `doctor --json` for reviewable repair
arguments, not automatic mutation. Explicit --schema is strict; exploratory
fallback requires --allow-schema-fallback.
