# Closing the ACUTE Loop: How BRICK Targets Should Declare Expected State

**Date:** 2026-03-26
**Status:** Research / Design Proposal

## The Problem

The ACUTE loop currently has a gap at the drift detection step. When pudl
runs `drift check` on a BRICK target, it needs two things:

- **Declared state**: what you said should be true
- **Live state**: what mu actually observed

The live side works — `mu observe --json` produces results, `pudl ingest-observe`
stores them in the catalog. But the declared side is empty for BRICK targets.

### Why It's Empty

pudl's drift checker builds "declared state" from **socket bindings** — cross-
definition references like `vpc_id: other_def.outputs.vpc_id`. These are
designed for wiring validation between resources, not for declaring what a
target's observed state should look like.

BRICK target definitions declare structure (toolchain, config, kind) but not
expected runtime state. The definition says "this is a k8s component with
3 replicas" but doesn't say "and when I observe it, I expect `state: converged`."

## How the Drift Checker Works Today

```go
// checker.go: builds declared state from socket bindings
declaredKeys := make(map[string]interface{})
for k, v := range def.SocketBindings {
    declaredKeys[k] = v  // v is a string like "other_def.outputs.vpc_id"
}
```

Then loads live state from the catalog (observe results or imports) and runs
a deep field-level comparison. The result is always "unknown" for BRICK targets
because they have no socket bindings.

## Possible Approaches

### Option A: `expected` Field on BRICK Targets

Add an optional `expected` field to `brick.#Target` that declares what
`mu observe` should return:

```cue
lint_go_vet: brick.#Target & {
    name:       "//lint/go-vet"
    kind:       "component"
    toolchain:  "lint"
    implements: "//interface/lint"
    config: {
        command: ["go", "vet", "./..."]
    }
    expected: {
        state: "converged"
    }
}
```

The drift checker would compare `expected` against the observe result:

```
Declared: {"state": "converged"}
Live:     {"state": "converged"}  →  clean
Live:     {"state": "drifted"}    →  drifted
```

**Pros:**
- Explicit and simple — you say exactly what you expect
- Works for any observe output format
- Easy to implement (one new field, one code path in checker.go)
- The `config` field stays separate from `expected` — config is for convergence,
  expected is for observation

**Cons:**
- Duplication: for many targets, `expected.state` is always `"converged"` —
  you'd be writing the same thing on every component
- Doesn't capture rich state (e.g., "I expect 3 healthy pods") without
  making the expected field complex

### Option B: Derive Expected State from Config

Instead of declaring expected state explicitly, derive it from the config
and toolchain. The assumption: if a target has been converged, its observe
state should be `"converged"`. If the config changed since last convergence,
it should be `"drifted"`.

Implementation: the drift checker compares the target's `config` hash
against the config hash recorded in the last build manifest. If they
differ, the target needs re-convergence.

```
Declared: hash(config) = abc123  (from definition)
Live:     hash(config) = abc123  (from last manifest) →  clean
Live:     hash(config) = def456  (from last manifest) →  drifted (config changed)
```

**Pros:**
- No extra field to write — drift detection is automatic
- Catches config drift without needing runtime observation
- Works for build targets (go, docker) that don't have observe

**Cons:**
- Doesn't detect runtime drift — only config drift. If someone manually
  edits a k8s resource, this approach wouldn't know
- Conflates "desired state changed" with "actual state drifted" — different
  problems requiring different responses
- Requires manifest ingestion to be working (it is, but it's another
  dependency in the chain)

### Option C: Implicit `state: "converged"` Default

The simplest approach: every BRICK component is expected to be `"converged"`
unless explicitly overridden. No new fields needed.

The drift checker would:
1. For BRICK targets, assume `declared = {"state": "converged"}`
2. Compare against the observe result
3. If observe result is `"drifted"` or `"unknown"`, report drift

```cue
// No expected field needed — converged is implied
lint_go_vet: brick.#Target & {
    name:       "//lint/go-vet"
    kind:       "component"
    toolchain:  "lint"
    config: { command: ["go", "vet", "./..."] }
}
```

For targets where you DON'T expect convergence (e.g., a monitoring-only
target), you'd override:

```cue
monitoring_check: brick.#Target & {
    name:       "//monitor/cpu"
    kind:       "component"
    toolchain:  "shell"
    expected: { state: "unknown" }  // don't flag this as drift
    config: { ... }
}
```

**Pros:**
- Zero boilerplate for the common case
- Works immediately for all existing BRICK definitions
- Still allows overrides for special cases
- The `expected` field only needs to exist for exceptions

**Cons:**
- Implicit behavior may surprise users — "why is my target drifted?"
- Doesn't support rich expected state without the override field

### Option D: Socket Bindings to Observe Results

Use pudl's existing socket binding mechanism to wire definitions to their
observe results. A BRICK target would bind its expected state to the output
of `mu observe`:

```cue
lint_go_vet: brick.#Target & {
    name:       "//lint/go-vet"
    kind:       "component"
    toolchain:  "lint"
    config: { command: ["go", "vet", "./..."] }

    // Socket binding: expected state comes from observe
    _observe_state: "converged"
}
```

The drift checker already knows how to compare socket bindings against
catalog entries. The change would be teaching it to recognize observe-result
bindings and compare them against the right catalog entry type.

**Pros:**
- Reuses existing infrastructure (socket bindings, drift comparison)
- Consistent with how other definitions declare expected state
- The discoverer already extracts these from CUE files

**Cons:**
- Socket bindings were designed for cross-definition wiring, not static
  value assertion — this is a semantic stretch
- The binding syntax is expression-based (`other.outputs.field`), not
  value-based (`"converged"`) — would need a new pattern

### Option E: Two-Phase Drift (Config + Observe)

Combine Options B and C: drift detection runs in two phases.

**Phase 1 — Config drift:** Compare the definition's `config` against the
config recorded in the last build manifest. If they differ, the target
needs re-convergence even if mu observe says "converged" (because the
definition has been updated but not yet applied).

**Phase 2 — Observe drift:** Compare `state: "converged"` (implicit default)
against the latest `mu observe` result. If mu reports "drifted", someone
or something changed the live resource out of band.

```
Phase 1: definition.config ≠ manifest.config  →  "config drifted"
Phase 2: observe.state ≠ "converged"          →  "runtime drifted"
Both clean                                    →  "clean"
```

**Pros:**
- Catches both kinds of drift: intent changed AND reality changed
- Clear separation of concerns
- Useful distinction for operators: "I need to re-deploy" vs "someone
  edited the cluster"

**Cons:**
- More complex implementation
- Requires both manifest ingestion and observe ingestion working
- More states to reason about

## Recommendation

**Start with Option C (implicit converged default), add Option A (explicit
`expected` field) as an override mechanism.**

This gives:
1. **Zero config for the common case** — every component is expected converged
2. **Explicit override** when you need different expectations
3. **Clean implementation** — one new optional field on `brick.#Target`,
   one code path in `checker.go`
4. **Future extensibility** — when Option E (two-phase) is needed, the
   `expected` field is already in place

### Implementation Sketch

**Schema change** (`brick.cue`):
```cue
#Target: {
    // ...existing fields...

    // Expected observe state. Defaults to {"state": "converged"} for
    // components. Set explicitly to override or to declare richer
    // expected state.
    expected?: {
        state: "converged" | "drifted" | "unknown" | *"converged"
        ...  // allow additional fields for rich state assertions
    }
}
```

**Drift checker change** (`checker.go`):
```go
// For BRICK targets, build declared state from expected field
// or use implicit {"state": "converged"} default
if isBrickTarget(def.SchemaRef) {
    declaredKeys = map[string]interface{}{"state": "converged"}
    if expected, ok := def.SocketBindings["expected"]; ok {
        // parse expected override
        declaredKeys = parseExpected(expected)
    }
}
```

**Observe result comparison:**
```
Declared: {"state": "converged"}
Live:     {"target": "//lint/go-vet", "state": "converged"}
           → extract: {"state": "converged"}
Result:   clean
```

### What This Enables

```bash
# 1. Define targets (expected: converged is implied)
pudl definition validate   # all pass

# 2. Observe live state
mu observe --json //lint/* | pudl ingest-observe

# 3. Check drift — now meaningful!
pudl drift check lint_go_vet
# Status: clean (observed converged, expected converged)

# 4. Someone breaks the linter...
# mu observe reports "drifted"
mu observe --json //lint/* | pudl ingest-observe

# 5. Drift check catches it
pudl drift check lint_go_vet
# Status: drifted
# Differences:
#   state: "converged" → "drifted"

# 6. Export and fix
pudl export-actions --definition lint_go_vet > fix.json
mu build --config fix.json
```

## Files to Modify

| File | Change |
|------|--------|
| `internal/importer/bootstrap/pudl/brick/brick.cue` | Add optional `expected` field to `#Target` |
| `internal/drift/checker.go` | Use BRICK expected state (default or explicit) as declared side |
| `internal/definition/discovery.go` | Extract `expected` field from BRICK definitions |
| `cmd/export_actions.go` | Exclude `expected` from exported config (it's not for mu) |
| `docs/mu-integration.md` | Document expected state and drift checking for BRICK targets |
| `docs/cli-reference.md` | Update drift check documentation |
