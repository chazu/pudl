# ADR-001: Projection as a first-class operation in pudl

**Status:** Proposed — **design accepted, execution deferred** pending a second
real call site. See §Trigger conditions.
**Date:** 2026-07-27
**Revised:** 2026-07-27, after checking the motivating claims against the repo.
The design is unchanged and remains the right shape. What changed is the
motivation: three of the four call sites the first draft cited do not exist in
pudl today, and the argument that decided Option B over Option C rested on an
architecture pudl does not have. §Context and §Options carry the corrections;
§Review findings records them.

## Summary

pudl would gain one function:

```go
func Project(v cue.Value, drop []cue.Path) (cue.Value, error)
```

It removes fields from a CUE value such that the result no longer constrains
them at all. No CUE fork. No new syntax in `.cue` files. Roughly 60 lines against
the existing `cue.Value` API, plus a property-test suite derived from four
algebraic laws.

The first draft's payoff was subtraction: four mechanisms currently maintained
separately collapsing into one operation. **That payoff is not available.** One
of the four exists, one is already solved by a different mechanism, and two
belong to subsystems pudl does not have. What remains is a well-scoped design
with a real test suite, ready to execute when a second caller appears.

---

## Context

### The gap in CUE (unchanged — this part is correct)

CUE's `&` is natural join. `{a: 1, b: int} & {b: 2, c: 3}` unifies shared labels
and carries disjoint ones, which is exactly ⋈ on a single tuple. This works and
we rely on it constantly.

What CUE lacks is the dual: **projection**, the operation that *removes* a
constraint. There is no expression taking `{a: 1, b: 2}` to `{a: 1}`.

Note that CUE's open structs are already doing this implicitly. `{a: 1}` doesn't
denote one struct — it denotes every struct with `a: 1` and anything whatever
elsewhere. Open-world semantics is existential quantification over unmentioned
labels, applied automatically. The gap is that we can't apply it *deliberately*
to a label that's already present.

In relational-algebra terms CUE has ⋈ and σ but not π. In logical terms it has ∧
but not ∃.

### What this actually costs us today: one thing

**Hidden fields aren't hidden, and it is already shaping our schemas.**

`_foo` fields are dropped at export, but they remain in the value and still
participate in unification. So:

```cue
{a: 1, _tmp: "x"} & {a: 1, _tmp: "y"}   // => _|_
```

A conflict on a field nobody can see, in values that print identically. That is
export-time erasure, not a quotient in the lattice.

This is not hypothetical in pudl. `~/.pudl/schema/pudl/git/git.cue:34-44` carries
two workarounds for it inside a single 10-line `_pudl` block:

```cue
#GitRepository: {
	_pudl: {
		schema_type:     "base"
		resource_type:   string | *"git.repository"   // widened so children can narrow
		identity_fields: ["name"]
		tracked_fields:  ["default_branch", "root_commit"]
		// Declared (optional) so platform specializations built with
		// `#Child: #GitRepository & {...}` may set it; `_pudl` is closed
		// inside a definition, so an undeclared field would be rejected.
		base_schema?: string
	}
```

`#GitHubRepository: #GitRepository & {_pudl: {resource_type: "git.repository.github", ...}}`
only works because the parent pre-widened `resource_type` to a defaulted
disjunction and pre-declared `base_schema` as optional. Every specialization-
friendly field on a base schema has to be anticipated this way.

**But note what this costs, precisely.** Each workaround is one token, idiomatic
CUE, and locally obvious. And `Project` would not remove it — the need here is
for a child to *override* `_pudl.resource_type`, not to drop it.
`Project(#GitRepository, ["_pudl","resource_type"]) & {_pudl: resource_type: "…"}`
is strictly more machinery than `string | *"git.repository"`. So the one real
cost we have is not one this proposal fixes.

### What the first draft claimed, and what is actually in the repo

| Claim | Reality |
|---|---|
| "Drift ignore-lists are projections written in a language that can't express projections… living in a separate artifact from the schema, free to drift from it" | **The mechanism does not exist.** No ignore-lists; no `ServerWritten` or `BuildMetadata` anywhere in the tree. `internal/acute/drift.go` is 29 lines of result structs. The real comparison is `cmd/run_inventory.go:125-139` `fieldsDiffer`, which iterates **desired** keys and ignores extra observed fields — ensure-present semantics. That is already a projection, performed structurally, in 15 lines. The differential path (`cmd/run_drift.go:50`) does not diff at all; the k8s plugin returns `matches`. |
| "View schemas are hand-maintained parallel copies… the subset of a model a definition is allowed to author" | **The definition layer was deleted.** World A — `internal/definition/`, sockets, standalone drift/status/export — was removed 2026-06-26 (`docs/vestige-sweep.md:56`). There is no definition authoring surface to derive. |
| "Method input resolution is a projection onto the method's declared input paths" | **pudl has no methods.** Models, methods and workflows were extracted to mu. |
| "Data artifact comparison… dropping timestamps and run IDs" | **Already solved, by the mechanism this ADR wants to build toward.** `_pudl.tracked_fields` is a declared projection controlling which changes create a new fact version, and it already lives in the schema next to what it masks — which is exactly the property the draft wanted to achieve for drift ("the ignore-list moves out of a separate config file and into the model definition"). |

So: of four call sites, one does not exist, one describes a mechanism that is not
there while the real code already projects, one belongs to a deleted subsystem,
and one is already handled.

### The unifying observation, corrected

The first draft's unifying claim was that hidden fields, view schemas, drift
masks, and Linda template formals in moor are all cylindrification wearing four
notations, and that "litelog already *has* the operator… implemented, working,
and in a different process," with "NDJSON at every seam."

The *algebraic* observation is correct and worth keeping: these are one
operation. The *architectural* premise is not pudl's architecture.

- pudl's Datalog is **in-process** — `internal/datalog`, compiled to SQL against
  the same SQLite handle. There is no pudl↔litelog seam.
- `docs/research/litelog-adoption.md` (2026-05-16, "Analysis Complete") already
  settled this: *"Is litelog a net positive replacement? **No.** … Should we
  depend on litelog? **No.**"* Different SQLite driver, different data model,
  different philosophy.
- `moor` exists at `~/dev/moor` but is referenced nowhere in pudl.

The one real seam is pudl↔mu, which does use NDJSON. Projection is not what is
wrong with it.

---

## Decision

**Accept the design. Do not build it yet.**

The design below is the right shape for this operation, and the analysis that
produced it — particularly the laws and the tier boundary — is worth keeping
intact rather than rediscovering. What it lacks is a second caller. Building a
shared abstraction for one arguable call site is the failure mode this repo's
`CLAUDE.md` guards against ("only make changes which directly contribute to the
specific task").

### Trigger conditions

Build `Project` when **any two** of these become true. Not before.

1. A model gains declared field masks for drift (the roadmap's convergence work
   would introduce this) and a second consumer needs the same masking.
2. An authoring-surface concept returns — a schema whose authorable subset must
   be derived from a larger one rather than hand-written.
3. `fieldsDiffer`'s ensure-present semantics prove insufficient and drift needs
   genuine two-sided equality on a projected heading.
4. A third caller appears for `tracked_fields`-style masking outside the fact
   store.

### What is worth doing now instead

Document the `_pudl` specialization idiom — `resource_type: string | *"default"`
plus `base_schema?: string` — in `docs/schema-authoring.md`, next to the
base-schema section. It is a one-token fix to a real problem that every schema
author will otherwise rediscover the hard way, and `git.cue:40-43` already
explains it in a comment that only readers of that file will find.

### Scope, if built: what Project handles

| Tier | Condition | Behavior | In scope |
|---|---|---|---|
| 0 | Dropped field is concrete, nothing symbolic depends on it | Field deletion | **Yes** |
| 1 | Dropped field ranges over a finite domain (enum, disjunction) | Enumerate, join the branches | **Yes** |
| 2 | Dropped field is symbolic in a decidable theory (bounds, regex) | Quantifier elimination | Later, maybe |
| 3 | Anything else (builtin calls, interpolation, arbitrary functions) | Error naming the blocking path | **Yes, as an error** |

Tier 0 covers the overwhelming majority of real uses and is free. Tier 1 costs a
loop. Tier 3 must fail loudly with the blocking path named — never silently
produce a value that looks right and isn't.

### Explicit non-goals

These are the things it would be easy to talk ourselves into. We are not building
them, and this stands whether or not `Project` is ever built.

- **No `\` operator in `.cue` files.** Files stay portable, `cue vet` still works
  in CI, the LSP still works, anyone can read them without our binary.
- **No unification variables or diagonals.** Logic variables in a configuration
  language are a large concept with a real learning curve, and they only buy
  equijoin, rename, and self-join. The Datalog layer owns joins. That is the
  division of labor: CUE handles the shape of a single value, Datalog handles
  relations between values.
- **No SMT integration, no quantifier elimination, no Presburger.** Tier 2 exists
  in the table so future-us knows the cliff is there, not as a work item.

The learning curve is one function with an obvious name. That is the whole point
of restricting scope this way.

### Laws (these are the test suite)

Projection is cylindrification in Tarski's cylindric algebra, which hands us the
correctness properties for free. Each becomes a property test:

| Law | Statement | Meaning |
|---|---|---|
| C0 | `Project(⊥, p) = ⊥` | Hiding a field of an impossible value stays impossible |
| C1 | `v ⊑ Project(v, p)` | Projection always widens; the result subsumes the input |
| C2 | `Project(v & Project(w,p), p) = Project(v,p) & Project(w,p)` | Projection pushes through join when the hidden path isn't shared |
| C3 | `Project(Project(v,a),b) = Project(Project(v,b),a)` | Order independence — projection is a set operation, not a sequence |
| — | `Project(Project(v,p),p) = Project(v,p)` | Idempotence |

C2 is the one that pays rent twice: it's both a correctness property and the
license for any future optimizer to reorder projection and unification.

### Closedness interaction

Projecting from a closed struct (`#Def`) must produce an **open** value.
Closedness says "no fields beyond these"; projection says "I no longer constrain
this field." The result must permit the dropped field to take any value, which
means the closedness assertion cannot survive intact.

This is a real semantic decision and it should be documented at the call site,
not discovered. If we need a closed result, closedness gets re-applied explicitly
at the boundary after projection.

Note that this interaction is *why* the `_pudl` problem in §Context is not solved
by projection: `_pudl` is closed inside a definition, and a child needs to
override a field within it, not widen the block.

---

## Options considered

### Option A: Fork CUE, add `\` and `?T` as syntax

| Dimension | Assessment |
|---|---|
| Complexity | High |
| Cost | Very high — own an evaluator that's under active rewrite upstream |
| Ecosystem | Lose `cue vet`, LSP, portability of `.cue` files |
| Cognitive load | High — two new concepts, one of them logic variables |

**Pros:** Full expressiveness. Projection composes inside config files.
**Cons:** The cost isn't the fork, it's owning the fork forever. Files stop being
handable to anyone outside our toolchain.

**Rejected**, unchanged. This assessment holds regardless of the corrections
below.

### Option B: Project() as a Go API operation

| Dimension | Assessment |
|---|---|
| Complexity | Low |
| Cost | ~60 lines + property tests |
| Ecosystem | Unaffected |
| Cognitive load | One function |

**Pros:** No language risk. Derived views become programs, which they already
were.
**Cons:** Can't write `#Pod \ status` as a schema expression. And — the
correction that matters — **there is currently no caller.**

**Accepted as the design, deferred as work.**

### Option C: Do nothing; keep projection where it already is

**Pros:** Zero new code. CUE stays fast and total. And the projections we
actually perform already work: `fieldsDiffer` projects structurally,
`tracked_fields` projects declaratively, Datalog rule bodies project variables
away.

**Cons (as first drafted):** *"Two value languages forever, with the seam between
them exactly where the current bugs live… Option C loses on the specific grounds
that the pudl↔litelog seam is where our time currently goes."*

**That reasoning does not hold.** There is no pudl↔litelog seam — pudl's Datalog
is in-process, and `docs/research/litelog-adoption.md` already ruled out taking
the dependency. With its deciding argument removed, **Option C is the correct
choice for now**, and Option B becomes the design we adopt when the trigger
conditions in §Decision are met.

---

## Prospective call sites

These are the sites that would justify building it. None exist today; each is
tied to work that is itself only proposed. Listed so the trigger conditions are
concrete, **not** as evidence of current cost.

**Drift check with declared masks.** If a model gains declared
`server_written` / `build_metadata` path lists — which
`docs/design/2026-07-27-swamp-parity-roadmap.md` does not currently propose —
the comparison becomes:

```go
o, _ := pudl.Project(observed, model.ServerWritten)
d, _ := pudl.Project(desired,  model.BuildMetadata)
drift := !o.Equals(d)
```

The mask would live next to the schema it masks. Note this is a *change in
semantics*, not only mechanism: today's `fieldsDiffer` is ensure-present
(observed may carry extra fields); this is two-sided equality on a common
heading. That change needs its own justification.

**A derived authoring surface.** If an authoring-surface concept returns, the
subset relationship becomes a fact the compiler holds (by C1) rather than a
comment:

```go
authorable, _ := pudl.Project(model, computedPaths)
```

**A third `tracked_fields` consumer.** `tracked_fields` is already a declared
projection; a second mechanism needing the same masking outside the fact store
would make the shared implementation worth extracting.

---

## Consequences

**If built, easier:**
- Schema and its masks live in the same artifact
- Subset relationships between schemas become machine-checkable
- One tested function instead of N ad-hoc maskings — *once there are N*

**If built, harder:**
- Tier-3 errors are a new failure mode users must learn. Error message quality
  matters more than usual — it must name the blocking path and say why.
- Projecting from closed definitions requires a conscious decision at each call
  site.
- Disjunctions can blow up under tier-1 enumeration. Needs a cardinality bound
  with a clear error above it.

**Cost of deferring:** low. The projections pudl performs today work. The one
real friction (`_pudl` specialization) has a one-token idiom that projection
would not improve. The analysis in this document does not expire.

**To revisit:**
- The trigger conditions in §Decision.
- If tier-3 errors turn out to be frequent rather than rare, that is the signal
  that residual values (an inspectable `∃(x: int) . {...}` rather than an error)
  are worth building. Not before.

---

## Action items

**Now:**

1. [ ] Document the `_pudl` specialization idiom (`string | *"default"`,
       `base_schema?: string`) in `docs/schema-authoring.md`

**When triggered** (see §Decision), in order:

2. [ ] Implement `Project` for tier 0 over `cue.Value`
3. [ ] Property tests for C0–C3 + idempotence
4. [ ] Tier-3 detection: walk dependencies of dropped paths, error with the
       blocking path named
5. [ ] Document the closedness rule at the API surface
6. [ ] Migrate the triggering call sites
7. [ ] Tier 1 (finite-domain enumeration) with a cardinality bound

---

## Review findings

Checked against the repo on 2026-07-27. Verified with `pudl schema list`,
`~/.pudl/schema/pudl/git/git.cue`, `internal/acute/`, `cmd/run_inventory.go`,
`cmd/run_drift.go`, `docs/vestige-sweep.md`, and
`docs/research/litelog-adoption.md`.

| # | Finding | Effect |
|---|---|---|
| 1 | Drift ignore-lists do not exist; `fieldsDiffer` (`cmd/run_inventory.go:125-139`) already projects structurally via ensure-present semantics | Call site removed; the masked-drift version reframed as prospective **and** as a semantic change |
| 2 | pudl has no methods — extracted to mu | Call site removed |
| 3 | The definition layer (World A) was deleted 2026-06-26 | "View schemas" call site removed |
| 4 | `_pudl.tracked_fields` is already a declared projection living beside its schema | Call site removed; noted that it already achieves the draft's stated goal |
| 5 | pudl's Datalog is in-process; `litelog-adoption.md` (2026-05-16) already ruled out the dependency; `moor` is unreferenced | The litelog framing removed; **Option C's deciding argument collapses, so Option C wins for now** |
| 6 | The hidden-field cost is real, and `git.cue:34-44` evidences it better than the draft did — but `Project` does not fix it (the need is override, not drop) | Premise kept, strengthened, and its limits stated |

**Standing weakness.** Even revised, nothing here is validated against a user
need. The strongest current evidence — the `_pudl` widening idiom — argues for a
paragraph of documentation, not 60 lines of Go. That is why the status is
deferred rather than rejected: the design is good and the analysis is durable;
the demand is not yet there.

---

## Appendix: why the theory was worth the detour

The algebra didn't produce a language. It produced three things:

1. **The knowledge that these are one operation.** Hidden fields, view schemas,
   drift masks, and Linda formals are cylindrification wearing four different
   notations. That remains true even though pudl currently instantiates only one
   of them — it is why the design can be written down now and executed later
   without rework.
2. **A test suite.** C0–C3 are Tarski's axioms, and they're exactly the
   properties a correct implementation must have. We didn't have to invent them.
3. **The location of the cliff.** Tier 3 is where projection stops being closed
   over CUE-representable values. Knowing that in advance means designing an
   error path rather than discovering the failure in production.

`&` is total and cheap; projection is where all the cost and all the
undecidability concentrate. That asymmetry is inherent to existential
quantification, not a defect in CUE — it just usually isn't this visible which
half you're paying for.
