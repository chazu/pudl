# Cross-resource wiring Phase 0, Phase 1, and observe-only orchestration

**Date:** 2026-08-05
**Design:** `docs/design/2026-07-28-cross-resource-value-wiring.md`
**Tracker:** `pudl-sj7`

## Outcome

PUDL now retains binding-bearing CUE models through discovery, elaborates a
standalone consumer from one exact successful observation, and coordinates an
explicit observe-only producer/consumer run set. Catalog lookup stays in PUDL;
the runtime `SystemModel` and generated mu projection contain only the resolved
plain value.

This is not the complete v1. Mutating run-set planning, canonical exact-plan
approval, mutation receipts, and strict sealed action routing remain Phase 2/3
work.

## Public and internal API

- `pudl run --max-observation-age <duration>` applies an optional run-level age
  bound to every pinned binding snapshot.
- `systemmodel.ModelTemplate` retains the authored `cue.Value`, owning context,
  origin metadata, input slots, and binding declarations. `Elaborate` and
  lossless `ElaborateJSON` unify fresh inputs and decode only after concrete CUE
  validation.
- `wiring.Resolver` selects exact schema-relative identities from a pinned
  successful snapshot, authorizes the source field with inherited
  `@pudl(binding=plain)`, traverses RFC 6901 object paths, rejects non-scalars,
  and returns typed/digested evidence with the concrete runtime model.
- Run reports now carry `report_version: 1` and plain binding evidence.
- Run reports carry structured completion status and optional `run_set_id`;
  `pudl run-set report [id]` retrieves the versioned orchestration record.
- Run audit rows have a structured `running`/`succeeded`/`failed`/`blocked`/
  `cancelled` completion status separate from drift verdicts.

## Catalog and observation invariants

- Latest reuse is exact to producer model and workspace and joins only to a
  `succeeded` owning run. Failed, unfinished, registration-only, cross-workspace,
  and legacy rows without provable status are ineligible.
- Current-run selection uses one exact producer run ID and never falls back to
  historical data.
- Observation ingest persists canonical identity JSON from the assigned schema's
  declared identity fields. Deduplicated older rows are enriched when metadata
  becomes available.
- The resolver pins one snapshot before inspecting members and never searches an
  older snapshot when the pinned one lacks or ambiguously contains the resource.
- Dry-run resolution opens an existing catalog read-only without migrations,
  backfills, view rebuilds, directory creation, or catalog writes.

## Observe-only run sets

- `pudl run-set <model> <model>...` is an exact named set; it does not discover
  or start omitted models. Every binding producer must be present.
- Binding and in-set `depends_on` edges are canonicalized through model aliases,
  topologically ordered producer-first, and lexically tie-broken. Duplicate
  models, self-dependencies, missing binding producers, and cycles fail preflight.
- A successful producer is registered only after its complete member lifecycle
  returns, so a consumer selects that exact run's successful snapshot and never
  falls back to history. A failed producer blocks transitive consumers while
  independent observe-only branches continue.
- Binding-derived dependency facts reconcile atomically under
  `binding:<consumer>` and coexist with declared and derived provenance.
- Run-set and member reports persist after each member. Synthetic blocked and
  preflight-failed members receive their own run IDs and durable reports.

## Sealed groundwork

The CUE schema and runtime projection now represent phase-owned sealed inputs
and converge-owned outputs, validate `@pudl(binding=sealed)`, and redact
provider paths from ordinary JSON. Existing ewe rendering lowers direct input
refs and modes. Cross-model sealed-source resolution, writable-ref ownership,
mandatory exact-plan approval, and mu strict action claims are intentionally not
claimed complete in this first slice.

## Verification

- `mise exec -- go test ./...`
- `mise exec -- go vet ./...`
- `mise exec -- go build ./...`
- `mise exec -- go run ./internal/skills/gen -check`
- `br lint`
- `git diff --check`

Focused coverage includes retained incomplete template discovery, inherited and
conflicting CUE classifications, exact input sets, wrong-type CUE rejection,
successful/current-run snapshot selection, workspace and run-status isolation,
age rejection, source-field denial, ambiguity, nested scalar projection,
identity enrichment on deduplication, provider-ref redaction, deterministic
run-set ordering, alias canonicalization, current-run selection, durable linked
reports, transitive blocking, independent-branch continuation, and read-only
catalog enforcement.
