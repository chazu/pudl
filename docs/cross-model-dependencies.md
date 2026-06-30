# Design: cross-model data dependencies

**Status:** **Phase 1 + Phase 2 BUILT (2026-06-30).** Phase 1:
`#SystemModel.depends_on` → reconciled `model_depends_on` facts → built-in
recursive rules (`depends_transitive` / `impacted_by` / `cyclic`) → `pudl query`,
plus `pudl query --list` / `--topo` and the opt-in `pudl run --check-upstream`.
Phase 2: `pudl model deps --derive` derives edges from desired↔produced identity
matching. The two Phase-1 leftovers are also closed: `pudl model deps` is the
no-run discovery pass (coverage gap), and query completion now lists derived
rule-head + EDB relations (discoverability). Validated end-to-end on a local k3d
cluster (real k8s convergence + derivation) and against a Docker container as a
fake remote host (inventory class). See
`implog/2026_06_30_cross_model_dependencies.md` and
`implog/2026_06_30_cross_model_deps_phase2.md`. Origin: mu
`docs/design/system-models/V1-BUILD-SPEC.md` §12, and the pudl-side convergence
work (`docs/system-models-build-status.md`).

## The problem

`pudl run <model>` is **single-instance**. A model declares `desired` state and
observes the world, but there is no way to express — or reason over — the fact
that **one model's state depends on another model's output**:

- a `network` model produces subnets/VPCs that a `compute` model's `desired`
  resources sit inside;
- a `cluster` model must converge before a `workloads` model can;
- a model's `checks` depend on inventory another model populates.

Two distinct consumers need to reason over these dependencies:

1. **The system** — to order runs, compute blast radius (what breaks if A
   changes), and flag downstream models when an upstream one drifts.
2. **The user/agent** — to answer "what depends on this model?" and "what does
   this model depend on?" before changing or deleting something.

Today neither is possible. No field on `#SystemModel` references *another model
instance* (the existing `relations?` field references Datalog **rules**, not
models — see §"`depends_on` vs. the existing `relations?` field"), and nothing
in the catalog records relationships between model instances.

## What this is NOT (and why)

This is **not** a revival of the retired World-A "socket" wiring. The V1 design
deliberately relocated every socket concern, and §12 records it:

- **Ordering** → the mu DAG (issue-ledger E5), not a pudl graph. Within a model,
  resource ordering is the plugin's job.
- **Relating** → pudl Datalog (P2), not in-body plugin calls or a declaration
  graph.
- **Value passing** (B's field = A's *generated* output, e.g. `subnet.vpcId ←
  vpc.id`) → the mu DAG / ewe `.result` threading — the deferred **ewe-converge**
  item (§7), whose revisit-trigger is a second/third pure-HTTP-CRUD converger.

So this design covers **reasoning over dependencies** (ordering, impact),
explicitly **not** author-visible declarative value interpolation at the
`desired` layer. That one genuinely-lost capability (Terraform-style
`${vpc.id}`) stays parked behind the ewe-converge trigger; it is a separate,
larger feature and YAGNI until a real consumer appears.

**Why this is not a socket resurrection** — and the spec authorizes it. The
retired socket concern was runtime **output→input value dataflow** between named
resources. `depends_on` declares only *that an edge exists* between two models;
it threads no value. mu V1-BUILD-SPEC §12 makes the same distinction and
prescribes exactly this shape:

> an inter-model `depends_on` relation (**declared on the model** and/or derived
> from shared schema/identity) is queryable via `pudl query` … Convergence work
> should add: (a) a way to **declare** a model's data dependencies, and (b) a
> relation to **reason over** them.

Declaring an edge on the model is mandated by the canonical spec; it is not the
declaration-layer wiring §12 says must stay retired (that was value dataflow).

## The principle: it belongs at the Datalog/catalog layer

Model instances are **already catalog records** — schema
`systemmodel.system_model`, identity `name`. On every live run,
`recordModelInstance` (`cmd/run_resolve.go:18`) ingests the instance under the
catalog target `//models/<name>` (stored target-column key `models/<name>`,
the `//` stripped — see the `definition`→`target` rename). The catalog is
already queryable via Datalog (`pudl query`), and pudl's Datalog engine already
supports **recursive rules** (semi-naive fixpoint, `internal/datalog/recursive.go`)
— exactly what transitive dependency closure and blast radius need.

So the home is a queryable **`model_depends_on` relation**, not a bespoke
in-memory graph. This is the cross-model analog of what sockets did within a
definition set, correctly relocated to the data layer where the design already
puts "relating."

## `depends_on` vs. the existing `relations?` field

`#SystemModel` already carries `relations?: [...string]`
(`internal/systemmodel/schema.cue:38-39`): the **RELATE** arm, a list of
**Datalog rule references** the model wants evaluated/derived. This new
`depends_on?: [...string]` is a deliberately separate field because it names a
**different kind of thing**:

| field | holds | answers |
|-------|-------|---------|
| `relations?` | names of **Datalog rules** to derive | "what derived relations does this model define/use?" |
| `depends_on?` | names of **other model instances** | "which models must produce state before this one?" |

They are orthogonal: one references rules, the other references models. A
model-to-model edge is *data*, not a rule, so it is not expressible as a
`relations?` entry. Keeping them distinct avoids overloading `relations?` with
two meanings. (If this distinction ever blurs, the alternative is to fold both
under one field with a tag — rejected here as more confusing, not less.)

## The naming/identity contract (load-bearing)

Everything below joins on **strings**, so the strings must line up exactly.
Pin these once:

- **What `depends_on` holds:** the **`name`** of another `#SystemModel`
  instance — the same string that model carries in its `name:` field and that
  `recordModelInstance` records as the subject of *its* edges. Not a definition
  name, not a `//models/...` target, not a `<name>:populate` sub-target. Using
  the instance `name` inherently excludes sub-targets (open question 4 resolved
  by construction).
- **The fact relation and its arg keys:** `model_depends_on` with named keys
  **`from`** (the declaring model) and **`to`** (the dependency). This matches
  the convention already used by the recursive evaluator's tests
  (`internal/datalog/recursive_test.go:69` uses `from`/`to`). Facts store args
  as a JSON object with meaningful keys — there are *no* positional args
  (`internal/database/facts.go:20`), so every rule body atom and every `pudl
  query` constraint below uses `from`/`to`.
- **Canonicalization at record time:** for each `depends_on` entry, pudl
  resolves it via `resolveModel` (`cmd/run_resolve.go`) and records the
  canonical instance `name`. If an entry does **not** resolve to a known model,
  pudl records the literal name (so forward references to a not-yet-created
  model still register the edge) but emits a **warning** in the run report. This
  prevents the silent transitive-join break where one model writes a def-name
  and another writes an instance-name for the same dependency.

## Proposed design

### Phase 1 — declared dependencies (buildable now, no infra)

**1. Declare.** Add an optional field to `#SystemModel`:

```cue
#SystemModel: {
    // ... existing fields, including relations?: [...string] ...
    // depends_on: NAMES of other #SystemModel instances whose output this
    // model's desired/observed state depends on. Model names (the dependency's
    // `name:` field), not value references — this expresses ordering/impact,
    // not value interpolation (see §"What this is NOT"), and not Datalog rule
    // references (that is `relations?`). Used to compute run order and blast
    // radius. Queryable via the model_depends_on Datalog relation.
    depends_on?: [...string]  // referenced model names
}
```

**2. Record as facts — reconcile, do not blind-append.** `recordModelInstance`
(`cmd/run_resolve.go`) runs on **every** live run. A naïve "emit one `AddFact`
per declared dependency" would create a **new** fact every run: the fact ID is
`SHA256(relation + args + valid_start + source)` and `valid_start` defaults to
`time.Now()` (`internal/database/facts.go:43,136`), so an unchanged edge gets a
fresh ID — and a fresh `current_facts` row — on each run, growing the store
without bound (worst exactly in a drift-polling loop). It also never removes an
edge the author *deletes* from `depends_on`, so blast-radius answers go stale.

Instead, **diff and reconcile** the declared edge set against the currently-valid
facts for this model (`from = <this model>`):

- declared **and** not currently valid → `AddFact model_depends_on {from,to}`;
- currently valid **and** no longer declared → `InvalidateFact` (valid-time end:
  the dependency stopped being true);
- declared **and** already valid → no-op.

This mirrors the content-hash-deduped catalog upsert `recordModelInstance`
already gets (via `ingestObserveOutput`), applied to facts. It makes re-runs idempotent (Finding: fact
churn) and keeps `current_facts` truthful when deps change (Finding: stale
edges), which also makes the bitemporal history meaningful ("what did B depend
on last week?" actually answerable).

**3. Reason over them (Datalog rules).** Ship rules in the pudl rules package.
The on-disk format is CUE top-level fields with `head`/`body` atoms and
`$`-prefixed variables (`internal/datalog/loader.go`), **not** the Prolog
notation sketched in earlier drafts. The real shipped form:

```cue
package rules

// transitive closure — base case reads the model_depends_on EDB directly
// (no redundant `depends` alias).
depends_transitive_base: {
    head: {rel: "depends_transitive", args: {from: "$A", to: "$B"}}
    body: [{rel: "model_depends_on", args: {from: "$A", to: "$B"}}]
}

// recursive step — evaluated by the fixpoint engine (semi-naive); shared $B
// is the equi-join.
depends_transitive_rec: {
    head: {rel: "depends_transitive", args: {from: "$A", to: "$C"}}
    body: [
        {rel: "model_depends_on",   args: {from: "$A", to: "$B"}},
        {rel: "depends_transitive", args: {from: "$B", to: "$C"}},
    ]
}

// reverse / blast radius: who is impacted when `changed` changes.
// Intention-revealing keys so the query direction is unambiguous.
impacted_by: {
    head: {rel: "impacted_by", args: {changed: "$X", impacted: "$A"}}
    body: [{rel: "depends_transitive", args: {from: "$A", to: "$X"}}]
}

// cycle surfacing — a model transitively depending on itself.
cyclic: {
    head: {rel: "cyclic", args: {model: "$A"}}
    body: [{rel: "depends_transitive", args: {from: "$A", to: "$A"}}]
}
```

These ship in `internal/importer/bootstrap/pudl/rules/` (a **new** non-definition
`.cue` file — `ParseRules` skips `#`-prefixed definitions, so the rules must be
plain top-level fields, not defs). `pudl init` copies the embedded bootstrap
into `~/.pudl/schema/pudl/rules/` (`CopyBootstrapSchemas`, `cmd/init.go`), so
every **freshly initialized** workspace gets
`depends_transitive`/`impacted_by`/`cyclic` for free. These shipped rules are
**canonical** (overwritten by `pudl init --force`, per the `62998c2` clobber
fix) — they are not meant to be edited in place. **Caveat for already-initialized
workspaces:** the gap-fill check (`ensureBasicSchemas` / `bootstrapChecks` in
`internal/importer/cue_schemas.go`) only re-copies *listed* files, so add the new
rules filename to `bootstrapChecks` (a one-line change) — otherwise pre-existing
workspaces get the rules only via `pudl init --force`.

Then the **actual** commands (positional `key=value` constraints — there is no
`--where` flag; `cmd/query.go` parses `key=value` positionally, and the keys are
the named arg keys above):

- **"what does `compute` depend on"** (direct + transitive) →
  `pudl query depends_transitive from=compute`
- **"what breaks if `network` changes"** (blast radius) →
  `pudl query impacted_by changed=network`
- **direct dependencies only** → `pudl query model_depends_on from=compute`
- **cycles** → `pudl query cyclic`

`--json` emits `[{"relation":"depends_transitive","args":{"from":"compute","to":"network"}}, ...]`
(`cmd/query.go`); the arg keys in the payload are exactly `from`/`to` (or
`changed`/`impacted`, `model`), so an agent can parse without guessing.

**4. Run ordering (a thin read-only helper, not an executor).** The motivating
use case is "what order do I run these in," and a bag of `(from,to)` pairs is not
that answer. pudl provides a small **read-only** helper that linearizes
`depends_transitive` into a topological order and **prints** it (it runs
nothing). On a cycle it errors and points at `cyclic`. Emitting an order is
*reporting*, squarely within pudl's role; it does **not** execute or re-run
anything (see §"What the system does"). Exact surface (a `pudl query --topo` flag
or a `pudl run --print-order` dry helper) is an implementation detail; the
contract is "prints a linear order or a clear cycle error."

**5. Cycle safety.** A dependency cycle has no valid run order. The fixpoint
evaluator terminates regardless (monotonic accumulation into a temp table with a
PK + `INSERT OR IGNORE`, `recursive.go`; proven by
`TestRecursiveFixpointTermination`), so `depends_transitive` is still
well-defined; the `cyclic` rule surfaces cycles for the user to fix. pudl reports
the cycle; it does not silently pick an order.

### Discoverability — CLOSED

Derived relations are **rule heads, not stored facts**, so they don't appear in
the fact table. Two fixes shipped: `pudl query --list` enumerates derived
rule-head relations (with arg keys) + EDB relations, and `completeRelations`
(`cmd/completion.go`) now folds rule heads + built-in EDB relations into shell
completion, so `depends_transitive` / `impacted_by` / `cyclic` complete even
before any fact exists. The `cmd/query.go` help example is backed by the real
shipped rules.

### Coverage — CLOSED by `pudl model deps`

`model_depends_on` edges are emitted from the **declaring** model's run, so a
model never run contributed no edge — an empty `impacted_by` meant "no recorded
dependents," not "provably none." **`pudl model deps`** closes this: it
reconciles every registered model's declared `depends_on` into facts **without
running them** (and `--derive` adds the Phase-2 edges). After a `pudl model deps`
the graph reflects the whole declared schema. The advisory phrasing on
`--check-upstream` is retained (it is still an advisory, but the graph is now
complete once the discovery pass has run).

### Phase 2 — derived dependencies (BUILT — `pudl model deps --derive`)

A model's dependency is often **latent** in its `desired`: model B's desired
resource references an identity that model A produces (e.g. B's Deployment names
a Namespace A declares). `pudl model deps --derive` derives
`model_depends_on(from:B, to:A)` without a manual declaration.

**Implementation note — why Go-side, not a Datalog join.** An early draft
assumed a Datalog join over a new EDB projection of desired identities. The
substrate makes that impractical: `desired` is **not** SQL-queryable (it lives in
the in-memory model / the stored record file, not a catalog column), and
`tags.model` is set only by the converge path. So derivation runs in Go over
resolved models and emits the **same** `model_depends_on` relation (under a
separate `derived:` fact source) — the Phase-1 rules are therefore unchanged, as
the original design required.

The match is **value-based**: `producedIdentities(A)` = A's desired resource
identities (top-level identity_fields / name|path|id, plus the k8s
`metadata.name` — scoped to metadata, so container/port names are not treated as
identities); `referencedValues(B)` = the string leaves of B's desired (skipping
structural type tags `kind`/`apiVersion`/`_schema`) minus B's own identities; an
edge is derived when they intersect, A ≠ B, and B does not already **declare** A
(declared wins; no duplicate). Because it is
value-based it is **heuristic** (a coincidental string equality can over-match),
so it is **opt-in** (`--derive`), **separately sourced** (auditable; never
corrupts the declared graph), and reconciled independently (`reconcileEdges`
scopes by fact `Source`). Validated on k3d: a `workloads` model with **no**
`depends_on` correctly derives an edge to `network` from its Deployment's
`metadata.namespace` referencing `network`'s Namespace.

## What the system does with the relation

pudl's role is to **make the dependencies queryable and reportable**, not to
become an orchestration engine — the charter is "pudl declares state; mu
executes" (`docs/concepts.md`, `docs/mu-integration.md`). Concretely, all
Phase-1 actions are **read-only advisories**:

- `pudl run` could, with an opt-in flag, **warn** when running a model whose
  upstreams are `drifted` (a stale-input guard), using `depends_transitive` +
  the per-target `status` (`unknown|drifted|converging|clean|failed`).
- Deletion safety: `pudl delete`/model removal can **warn** when `impacted_by`
  is non-empty (subject to the coverage caveat — phrase as "recorded
  dependents").
- The topological **order helper** (Phase 1 step 4) prints an order; it does not
  run anything.

**Re-running downstream models is out of scope for pudl.** Automatically
re-triggering `impacted_by` models after an upstream reaches `clean` is
**orchestration** — the mu DAG's job (ordering → mu DAG, E5), or an external
scheduler reading the relation. pudl emits `impacted_by` and the topo order; a
consumer outside pudl decides whether and how to act. There is no
`pudl run --with-downstream`; proposing one would stake a claim on orchestration
inside pudl's CLI that the charter forbids.

## Phasing / revisit-triggers

| Item | When |
|------|------|
| ✅ Phase 1: `depends_on` field + reconciled `model_depends_on` facts + the 3 rules + topo helper + `pudl query` | DONE 2026-06-30 |
| ✅ Stale-upstream warning (`pudl run --check-upstream`) | DONE 2026-06-30 |
| ✅ Coverage: `pudl model deps` no-run discovery pass | DONE 2026-06-30 |
| ✅ Phase 2: derived dependencies (`pudl model deps --derive`, Go-side value match) | DONE 2026-06-30 |
| Deletion-safety warning | future — `pudl delete` is generic catalog-entry deletion; a model-aware warn is a separate, small follow-up |
| Value threading (`${vpc.id}`) | **NOT here** — the ewe-converge item (§7), its own trigger |
| Cross-model run re-triggering / scheduling | **NOT pudl** — mu DAG or an external scheduler consuming the relation |

## Open questions

1. **Reference granularity.** `depends_on: ["network"]` by model name is
   simplest and is the pinned contract (§"naming/identity contract"). Referencing
   a *specific resource* a model produces (finer than model-level) edges toward
   value threading (out of scope) — keep it model-level. **Resolved:
   model-level, by instance `name`.**
2. **Where rules ship.** Built-in (`bootstrap/pudl/rules/` → copied to
   `~/.pudl/schema/pudl/rules/` by `pudl init`) so every workspace gets
   `depends_transitive`/`impacted_by` for free. **Resolved: built-in** — this is
   core convergence reasoning, not a user rule.
3. **Graph answer ergonomics.** A topo-sorted run order is a sequence, not a
   relation. **Resolved:** ship the read-only topo helper in Phase 1 (step 4) —
   the raw relation alone does not satisfy the motivating "what order" use case,
   and a helper that *prints* an order (or a cycle error) stays within reporting,
   not execution.
4. **Self-reference / `:populate` sub-target.** `model_depends_on` is between
   top-level model **names**; the `//models/<name>:populate` target is an
   internal phase, not a dependency edge. **Resolved by construction:** edges use
   the instance `name`, which never carries a `:populate` suffix.
