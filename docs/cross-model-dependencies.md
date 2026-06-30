# Design: cross-model data dependencies

**Status:** design proposal (not yet built). Origin: mu
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
   changes), and re-run downstream models when an upstream one drifts/converges.
2. **The user/agent** — to answer "what depends on this model?" and "what does
   this model depend on?" before changing or deleting something.

Today neither is possible: there are no cross-model references anywhere in
`#SystemModel`, and nothing in the catalog records relationships between model
instances.

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

So this design covers **reasoning over dependencies** (ordering, impact,
re-triggering), explicitly **not** author-visible declarative value
interpolation at the `desired` layer. That one genuinely-lost capability
(Terraform-style `${vpc.id}`) stays parked behind the ewe-converge trigger; it is
a separate, larger feature and YAGNI until a real consumer appears.

## The principle: it belongs at the Datalog/catalog layer

Model instances are **already catalog records** — schema
`systemmodel.system_model`, identity `name`, target `models/<name>` (see the
`definition`→`target` rename). The catalog is already queryable via Datalog
(`pudl query`), and pudl's Datalog engine already supports **recursive rules**
(semi-naive fixpoint, `internal/datalog/recursive.go`) — exactly what transitive
dependency closure and blast radius need.

So the home is a queryable **`model_depends_on` relation**, not a bespoke
in-memory graph. This is the cross-model analog of what sockets did within a
definition set, correctly relocated to the data layer where the design already
puts "relating."

## Proposed design

### Phase 1 — declared dependencies (buildable now, no infra)

**1. Declare.** Add an optional field to `#SystemModel`:

```cue
#SystemModel: {
    // ... existing fields ...
    // depends_on: other #SystemModel instances whose output this model's
    // desired/observed state depends on. Names, not value references — this
    // expresses ordering/impact, not value interpolation (see §"What this is
    // NOT"). Used to order runs, compute blast radius, and re-trigger
    // downstream models. Queryable via the model_depends_on Datalog relation.
    depends_on?: [...string]  // referenced model names
}
```

**2. Record as facts.** `recordModelInstance` (`cmd/run_resolve.go`) already
upserts the instance into the catalog on every run. Extend it to also emit one
fact per declared dependency:

```
model_depends_on(<this-model>, <dep-model>)
```

into the bitemporal fact store (`AddFact`). Facts (not just catalog columns)
because the dependency is a relationship between two entities, and the fact store
+ Datalog is pudl's join/relate substrate. Bitemporality means the dependency
graph is itself time-travellable (what did B depend on last week?).

**3. Reason over them (Datalog rules).** Ship rules in the pudl rules package:

```
# direct dependency (EDB, from the emitted facts)
depends(A, B)            :- model_depends_on(A, B).

# transitive closure — recursive, evaluated by the fixpoint engine
depends_transitive(A, B) :- depends(A, B).
depends_transitive(A, C) :- depends(A, B), depends_transitive(B, C).

# reverse / blast radius: who depends on X (directly or transitively)
impacted_by(X, A)        :- depends_transitive(A, X).
```

Then:
- **"what does B depend on"** → `pudl query depends_transitive --where A=B`
- **"what breaks if A changes"** (blast radius) → `pudl query impacted_by --where X=A`
- **run ordering** → a topological read of `depends_transitive` (a small CLI
  helper, or left to the consumer; pudl provides the relation, not an executor).

**4. Cycle safety.** A dependency cycle has no valid run order. The fixpoint
evaluator terminates regardless (it reaches a fixed point), so `depends_transitive`
is still well-defined; a `cyclic(A) :- depends_transitive(A, A)` rule surfaces
cycles for the user to fix. pudl reports the cycle; it does not silently pick an
order.

### Phase 2 — derived dependencies (optional, later)

Phase 1 requires the author to declare `depends_on`. A model's dependency is
often **latent** in its `desired`: model B's desired resource references an
identity that model A *produces* (already in the catalog, tagged with A's
target). That can be **derived** by a Datalog join — B.desired.X's identity
matches a catalog resource produced under `models/A` — yielding
`model_depends_on(B, A)` without a manual declaration.

Defer this: it needs resource-level identity matching between `desired` entries
and produced catalog rows, and the declared form (Phase 1) covers the explicit
cases first. Derivation is additive — it produces the same relation, so the
rules in Phase 1 are unchanged.

## What the system does with the relation

pudl's role is to **make the dependencies queryable**, not to become an
orchestration engine. Concretely:

- `pudl run` could, with an opt-in flag, **warn** when running a model whose
  upstreams are `drifted` (a stale-input guard), using `depends_transitive` +
  the per-target `status`.
- A future `pudl run --with-downstream` (or an external scheduler reading the
  relation) could re-trigger `impacted_by` models after a converge. This is a
  policy layer on top of the relation — design it only when there's a consumer.
- Deletion safety: `pudl delete`/model removal can refuse (or warn) when
  `impacted_by` is non-empty.

Keep the reasoning in Datalog and the *acting* thin and opt-in — the same
division that keeps pudl declaring and mu executing.

## Phasing / revisit-triggers

| Item | When |
|------|------|
| Phase 1: `depends_on` field + `model_depends_on` facts + the 3 rules + `pudl query` | the **first model whose run must be sequenced after, or re-triggered by, another model's output** (the §12 revisit-trigger) |
| Stale-upstream warning on `pudl run` | once Phase 1 exists and a real multi-model topology is in use |
| Phase 2: derived dependencies (identity join) | when manual `depends_on` becomes tedious / error-prone across many models |
| Value threading (`${vpc.id}`) | **NOT here** — the ewe-converge item (§7), its own trigger |

## Open questions

1. **Reference syntax.** `depends_on: ["network"]` by model name is simplest.
   Do we need to reference a *specific resource* a model produces (finer than
   model-level)? Model-level is enough for ordering/impact; resource-level edges
   toward value threading (out of scope) — keep it model-level.
2. **Where rules ship.** Built-in (`~/.pudl/schema/pudl/rules/`) so every
   workspace gets `depends_transitive`/`impacted_by` for free, vs. repo-scoped.
   Built-in is right — this is core convergence reasoning, not a user rule.
3. **`pudl query` ergonomics for a graph answer.** A topo-sorted run order is a
   sequence, not a relation. Decide whether pudl emits the ordering (a small
   helper over `depends_transitive`) or returns the relation and leaves ordering
   to the caller. Lean: provide the relation; add an ordering helper only if a
   consumer needs it.
4. **Self-reference / the model's own `:populate` sub-target.** `model_depends_on`
   is between top-level model names; the `models/<name>:populate` target is an
   internal phase, not a dependency edge — exclude it.
