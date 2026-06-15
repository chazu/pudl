# System changes to support decomposed resources (the git-repository example)

**Priority:** Medium
**Status:** Proposed
**Date:** 2026-06-15

## Summary

We want a family of built-in schemas describing git repositories: a
platform-agnostic `GitRepository` at the root, with platform-specific
specializations (e.g. a hosted-platform variant, and a policy variant on top of
that) inheriting from it. Data is pulled as JSON from an external CLI and
imported.

Designing this surfaced a gap between how pudl models a *single* resource and
how it would model a resource that decomposes into **several related resources**
(a repository, its branches, its remotes). The schema language (CUE) is enough
to describe the shapes, but three system-level capabilities are missing or
unspecified once you split one logical thing into multiple tracked resources:

1. A first-class **component vs. schema** distinction, so reusable shapes can be
   composed into schemas without themselves becoming tracked resources.
2. **Ingestion fan-out** — turning one imported blob into multiple catalog
   entries, each stamped with a foreign key back to its parent.
3. **Lifecycle reconciliation** — retracting child resources that have
   disappeared upstream, since pudl treats *absence* as no signal.

This doc records the decisions reached and the concrete changes each implies. It
uses the git-repository schemas as the motivating example but the mechanisms are
general.

## Motivating shapes

```cue
// component — no _pudl. A reusable shape, lives INSIDE a repository.
#GitRemote: {
    name: string          // "origin"
    url:  string
    push_url?: string
}

// tracked resource — has _pudl.
#GitRepository: {
    _pudl: {
        schema_type:     "base"
        resource_type:   "git.repository"
        identity_fields: ["name"]                      // name-only identity
        tracked_fields:  ["default_branch", "root_commit"]
    }
    name:           string          // fully-qualified path, for global uniqueness
    default_branch: string
    bare?:          bool
    root_commit?:   string          // first (parentless) commit; OPTIONAL ⇒ tracked, not identity
    remotes: [...#GitRemote]        // zero or more, inline component
}

// tracked resource — its OWN _pudl resource, not inline.
#GitBranch: {
    _pudl: {
        schema_type:     "base"
        resource_type:   "git.branch"
        identity_fields: ["repo", "name"]              // composite; repo is the FK
        tracked_fields:  ["sha"]
    }
    repo: string          // == the parent GitRepository's `name`
    name: string          // "main", "release/1.2"
    sha:  string          // current tip
}
```

Two structural decisions are baked in here and explained below: `GitRemote` is an
**inline component**, `GitBranch` is a **separate tracked resource**.

## Decisions

### D1. `_pudl` metadata is the schema/component boundary

A CUE definition **with** a `_pudl` block is a *schema* (a tracked resource type,
eligible for inference and identity). A definition **without** `_pudl` is a
*component* — a reusable shape meant to be embedded in a schema, inert to
inference.

This matches pudl's de-facto behavior today but is not currently a rule:
`internal/validator/cue_loader.go` (`createModuleFromInstance`) registers *every*
`#`-prefixed definition as a schema, and `_pudl` is decoded only if present. A
component like `#GitRemote` is registered as a candidate schema, scores no
inference boosts (the boosts in `internal/inference/heuristics.go` gate on
`len(meta.IdentityFields) > 0` / `len(meta.TrackedFields) > 0`), and so is inert
in practice — but it is still a visible "schema" in listings and reinference.

**Change:** make the boundary explicit. In `createModuleFromInstance`, skip
registering definitions that carry no `_pudl` block (≈ one-line filter), or
register them in a separate "components" set excluded from inference candidates.
Either way: components stop appearing as phantom schemas, and the authoring model
("compose schemas from components") becomes a real, enforced concept rather than
an accident of scoring.

### D2. Identity fields must be present-on-every-instance; optional ⇒ tracked

`root_commit` is attractive as identity (stable across clones, forks, and
re-hosting) but it is **optional**: empty repositories have none, and a repo can
have multiple root commits. An identity field whose value is sometimes absent
changes the `resource_id` hash between imports and **splits one logical resource
into two**. Therefore:

> A field may be in `identity_fields` only if it is guaranteed present on every
> instance. Optional-but-stable fields belong in `tracked_fields`.

So `GitRepository` identity is `["name"]` and `root_commit` is tracked. `name`
must be the **fully-qualified path** (git assigns no inherent name), so that the
identity is globally unique. This also satisfies the existing family-identity
invariant cleanly: the platform specializations inherit `identity_fields:
["name"]` unchanged and may only tighten the `name` constraint (e.g. a pattern),
never change the field set.

No code change — this is a design rule for authoring the schemas, and the
existing `pudl doctor` Identity Fields check already backstops divergence within
a family.

### D3. `GitRemote` is an inline component; `GitBranch` is a separate resource

These are deliberately different, and the difference is the crux of the doc.

- **`GitRemote` inline** because a remote has no independent lifecycle worth
  tracking and no need for its own version history. Re-importing the repository
  simply replaces the whole `remotes` array. No fan-out, no foreign key, no
  referential-integrity gap, no reconcile step.

- **`GitBranch` separate** because we *do* want independent bitemporal history
  per branch — to track how each branch tip (`sha`) moves over time, which an
  inline array cannot give us (the array is one value on the repo, versioned as a
  whole). This benefit is the entire justification for the machinery in D4–D6.
  If per-branch tip history is not actually needed, `GitBranch` should be an
  inline component too, and D4–D6 disappear.

## Required system changes (only if D3 keeps `GitBranch` separate)

### C1. Ingestion fan-out

Branches arrive from a *separate* source call returning an array, and each
element does **not** carry the repository identity. Import must:

1. Import that array as separate catalog entries (pudl already handles JSON
   arrays / NDJSON).
2. **Stamp each branch with `repo: "<the repository's canonical name>"`** — using
   the exact string that is the `GitRepository`'s identity. If the stamped value
   doesn't match the repo's `name` byte-for-byte, the join silently breaks.

This is an extraction-side responsibility. It implies either an import-time
transform/mapping step that injects the parent key, or a documented extraction
convention the caller follows before importing. Decide which; today pudl has no
built-in "split this blob and stamp a parent FK" primitive.

### C2. Composite identity + inference for child resources

`GitBranch` identity is `["repo", "name"]`. The +0.5 identity boost fires only
when **both** are present in the data, so C1's stamping is a prerequisite for the
branch to even classify as a `GitBranch` (rather than falling to catchall). Worth
a distinct `resource_type: "git.branch"` and a well-named import origin (e.g.
`branches.ndjson`) so the origin-keyword boost helps disambiguate the branch
shape from any future sibling shapes.

### C3. Referential integrity is not enforceable by CUE

CUE validates each entry in isolation; it cannot assert that a branch's `repo`
points at a real `GitRepository`. An orphaned or typo'd `repo` is accepted
silently. **Backstop with a datalog rule or a `doctor` check**: "branches whose
`repo` has no matching repository." Cheap to add and the natural home for this
class of cross-resource invariant.

### C4. Deletion/retraction reconciliation (the one that actually bites)

pudl's fact store treats **absence as not-a-signal**: if a branch is deleted
upstream, the next import simply omits it — pudl does **not** auto-retract it, so
the stale branch lingers in `current_facts` as if it still exists. Per-child
resources therefore need an explicit **reconcile step**: after importing the
current set of branches for a repo, diff against the existing branches for that
repo and `RetractFact`/invalidate the ones that disappeared.

This is the real cost of D3. Inline components (D3 for remotes) avoid it for
free, because replacing the array drops removed entries implicitly. Options:

- A generic import mode: "these N entries are the complete set for parent P;
  retract any current child of P not in this set." (Reusable beyond git.)
- A git-specific reconcile command.
- Accept staleness and document it.

Recommend the generic "complete-set import for a parent scope" mode, since the
same problem recurs for any decomposed resource.

## Non-impacts

- **The platform specialization family is unaffected.** It sits only on
  `GitRepository` (`#PlatformRepository: #GitRepository & {...}`, a policy
  variant on top of that). Repository and branch are independent inheritance
  roots, so their `identity_fields` lists are independent — no cross-family
  invariant conflict.
- **`default_branch` stays a plain field** on the repository (a name). "Is this
  branch the default?" is derived via the join `repo.default_branch ==
  branch.name`, not modeled as a flag on the branch.

## Sequencing

1. **D1** (component/schema filter) — small, independently useful, unblocks clean
   authoring. Do first.
2. Author the schemas per D2/D3 (CUE only, no engine change).
3. If `GitBranch` stays separate: **C1** (fan-out + FK stamping) and **C4**
   (reconcile) are the load-bearing changes; **C3** (integrity check) follows.
   **C2** is mostly authoring + an origin convention.

## Open questions

- Is per-branch tip history (D3) actually wanted now, or is inline-component the
  right call until there's a concrete need? This single decision gates all of
  C1–C4.
- For C1/C4: build a general "complete-set import for a parent scope" primitive,
  or handle git specifically? The general primitive is more work but pays off for
  every future decomposed resource.
