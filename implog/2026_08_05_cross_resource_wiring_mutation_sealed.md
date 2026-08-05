# Cross-resource wiring mutation approval and sealed execution

**Date:** 2026-08-05
**Design:** `docs/design/2026-07-28-cross-resource-value-wiring.md`
**Tracker:** `pudl-sj7`
**Mu dependency:** `a334872` plus test hardening `6e757b8` on
`codex/pudl-strict-sealed-routing`

## Outcome

PUDL now coordinates exact mutating run sets. It finishes read-only member
work, constructs a canonical complete mutation identity, optionally or
mandatorily pauses for approval, reconstructs the plan from pinned evidence,
and executes in producer-first order. A changed model, plugin/action plan,
snapshot, policy, or option makes the approval stale without mutation.

Sealed references remain metadata in PUDL. Cross-model sources inherit the
producer-owned provider reference and store mode; consumers own only their
local name and delivery mode. Generated mu targets opt into strict action
routing and forward the workspace's writable-ref policy. Provider values are
resolved only by mu at execution time.

## Public API

- `pudl run-set <models...> --converge` plans all mutating members before the
  first apply and executes without approval only when policy permits.
- `--require-approval` persists the exact plan and returns at the boundary.
  Any converge-owned sealed output forces the same boundary during a mutating
  run even when the flag is absent.
- `pudl run-set resume <run-set-id>` reconstructs and digest-checks the complete
  plan before atomically approving it and beginning execution.
- `pudl run-set reject <run-set-id>` terminates a pending plan without mutation.
- `pudl run-set report [run-set-id]` returns the versioned orchestration record;
  linked member reports contain plain provenance, metadata-only sealed
  provenance, action claims, and mutation receipts.
- Workspace `secrets.writable_refs` is the sole PUDL write-policy authority;
  omitted and explicit empty policies remain distinct.

## Safety and persistence

- Pending approval, member running reports, and retained snapshots commit in one
  SQLite transaction. Approval compare-and-set and its running report also
  commit atomically.
- Durable approval JSON stores reference and policy fingerprints, never provider
  paths. Live interactive review is the intentional surface that shows complete
  destinations and store modes.
- Mu plan JSON is canonicalized before hashing: redundant action keys and only
  the current temporary workspace identity are removed. Semantic action, mode,
  config, plugin, or source changes still invalidate approval.
- The first failed mutation blocks dependent members and cancels independent
  unstarted members. Before every external apply, PUDL durably records an
  unverified mutation intent; only a committed manifest receipt clears it.
  Apply or receipt failure therefore becomes `needs-verification`/`unknown`,
  receives no automatic retry, and stops later mutations.
- Sealed execution errors are scrubbed against the model's operational refs
  before terminal output or report persistence.
- Unresolved plain bindings persist and render a typed, value-free selector,
  stable resolution code, and failure message in member and standalone JSON
  diagnostics.

## Mu boundary

Mu commit `a334872` adds `sealed_routing: "strict"`, target/action output modes,
complete structured JSON plans, and delayed sealed-input resolution. Strict
planning rejects implicit fan-out, undeclared or unused claims, ref/mode
changes, and outputs without exactly one producer. A real fake-provider test
proves planning does not resolve a value, execution performs resolve/store with
the requested mode, and no secret reaches stdout. Follow-up `6e757b8` proves
write policy is rechecked at execution after graph tampering and that provider
references and secret values remain absent from the manifest.

## Acceptance audit

The design's 21 numbered criteria map to deterministic tests as follows:

| Criteria | Authoritative coverage |
| --- | --- |
| 1–5 | `internal/wiring/resolver_test.go`, `internal/systemmodel/template_test.go`, and the current-run run-set fixture cover exact scalar selection, missing/ambiguous sources, pinned/latest snapshots, age, and final CUE typing. |
| 6–8 | Run-set integration persists typed unresolved selectors; generated mu config proves concrete input with no authoring metadata; the mu boundary has no catalog dependency or lazy PUDL lookup. |
| 9–12 | Cross-model sealed integration plus mu's fake provider, strict-routing, non-leakage, and plan/write-time policy tests cover metadata-only reference transport and both enforcement points. |
| 13–15 | Approval/resume/reject, changed-plan, completed-partial-state, global fail-fast, blocked, and cancelled integration tests cover the mutation boundary. |
| 16–18 | Producer-owned sealed resolution, model-schema policy rejection, declaration/classification tests, and exact action-claim tests cover ownership and least privilege. |
| 19–20 | Binding fact reconciliation and persisted versioned member/run-set reports cover durable provenance, redaction, and receipts. |
| 21 | Pinned snapshot tests, atomic approval/retention tests, a repeated concurrent approval race, and write-ahead mutation-intent tests cover redirection, pruning, CAS, and crash uncertainty. |

The release matrix also has explicit coverage for workspace isolation,
no-fallback selection, missing and non-leaf projections, direct-ref graph
non-edges, model-owned policy rejection, and execution-time write-policy
defense. The former populate-output contract row is resolved below. Schema
tests reject it for both populate arms, while an observe-only integration test
proves a dormant converge output causes no plan or apply.

## Final populate/output contract

An action-backed populate can produce records and a sealed output in the same
mu action. The accepted design requires those records before complete exact-plan
approval while also forbidding the action's sealed write before approval. V1
therefore makes sealed outputs converge-only. Populate may consume sealed
inputs, but its CUE arms, decoded Go type, and generated mu project expose no
sealed-output path. Observe-only runs may inspect dormant converge declarations
without mutation; mutating runs that can execute them require exact approval.

## Verification

- `mise exec -- go test ./...`
- `CGO_ENABLED=0 mise exec -- go test ./...`
- `mise exec -- go vet ./...`
- `mise exec -- go build ./...`
- `mise exec -- go run ./internal/skills/gen -check`
- `br lint`
- `git diff --check`
- mu: `mise run test`
- mu: `cue vet internal/config/schema.cue`
- mu: `go run ./cmd/mu verify` (280 blobs healthy)
