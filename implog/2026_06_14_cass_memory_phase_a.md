# cass-memory substrate — Phase A

Date: 2026-06-14

## Summary

First slice of the cass-memory (cm/ACE) substrate plan
(`docs/cass-memory-substrate-plan.md`). Phase A is the zero-engine-risk foundation:
the `#Feedback` schema, the canonical fact-write command, the maturity-transition
command, and the `pudl mu` namespace tidy-up. No datalog engine changes (those are
Phase B+).

Guiding invariant held throughout: **pudl stores and scores; it never reflects or
decides.** `promote` moves a status flag and records a pointer — it does not judge
or synthesize.

## Changes

### Schema
- `internal/importer/bootstrap/pudl/nous/nous.cue` — added `#Feedback`
  (`target`, `verdict` enum helpful|harmful|neutral, optional `outcome`
  success|failure, `source`, optional `note`). Append-only reinforcement signal;
  corroboration preserved as distinct facts, not aggregated. Scoring/weighting is
  deferred to read-time (Phase C decay view), not applied at ingest.

### Commands (`cmd/`)
- **`pudl facts add`** (new, `cmd/facts_write.go`) — the one canonical low-level
  fact write. Validates `--args` is a JSON object; rejects reserved relations
  (`database.IsReservedRelation`); optional `--schema` validates args via
  `validator.ValidationService` (hard-fail on invalid). Flags: `--relation`
  (required), `--args` (required), `--source` (default OS user), `--schema`.
- **`pudl facts promote <id>`** (new, `cmd/facts_write.go`) — maturity state
  machine over `#Observation.status`. Runs the read-check-write under
  `CatalogDB.WithFactTx` (BEGIN IMMEDIATE), so concurrent promotions cannot race.
  Legal transitions: `raw → reviewed|rejected`, `reviewed → promoted|rejected`
  (promoted/rejected terminal). Invalidates the prior version and appends a new
  one with the updated status (+ optional `promotedTo` via `--rule`, only valid
  with `--to promoted`). Source/relation preserved; differing args yield a new ID.
- **`pudl facts observe`** — moved from top-level `pudl observe` to under the
  `facts` group (sugar for `facts add --relation observation`). `cmd/observe.go`
  reparented; now uses the shared `defaultFactSource()` helper; dropped the unused
  `os/user` import.
- **`pudl mu`** (new parent, `cmd/mu.go`) — groups the three mu-bridge commands.
  `export-actions`, `ingest-observe`, `ingest-manifest` reparented from top-level
  to `pudl mu export-actions` / `pudl mu ingest-observe` / `pudl mu ingest-manifest`
  (their `rootCmd.AddCommand` registrations removed).

### Net agent-facing surface
New under the existing `facts` group: `facts add`, `facts promote` (plus moved
`facts observe`). Moved under new `mu` group: the three bridge commands. **Zero new
top-level verbs.** One canonical way to assert a fact (`facts add`); data-lake
(`import`) and mu-bridge (`mu …`) doors remain distinct.

## Public API / CLI

```
pudl facts add --relation R --args '<json-object>' [--source S] [--schema pkg.#Def]
pudl facts observe <description> [--kind K] [--scope repo:path] [--source S]
pudl facts promote <id> --to reviewed|promoted|rejected [--rule <ref>]
pudl mu export-actions | ingest-observe | ingest-manifest
```

Internal helper added: `cmd.defaultFactSource()` (current OS username or "human").

## Verification

- `CGO_ENABLED=0 go build ./...` clean; `CGO_ENABLED=0 go test ./...` all green.
- Manual lifecycle: feedback add; bad-JSON rejected; reserved relation rejected;
  observe sugar; `raw→promoted` blocked as illegal; `raw→reviewed→promoted --rule`
  applies and sets `promotedTo`; superseded versions correctly rejected (TOCTOU);
  current view shows exactly the live version after each promote.

## Fast-follow (same day): auto-validation + docs

- **Auto strict validation of known agent relations.** Rather than lean on the
  catalog `CascadeValidator` (catchall-fallback semantics, on-disk dependency), added
  `importer.ValidateAgainstBootstrapDef(embedFile, defName, data)` — strict CUE
  unification against the *embedded* definition (`Unify(...).Validate(cue.Concrete(true))`),
  self-contained, no disk dependency. `cmd` registry `bootstrapRelationSchemas`
  maps `observation`→`#Observation`, `feedback`→`#Feedback`; shared
  `validateKnownRelation()` is called by both `facts add` (when no `--schema`) and
  `facts observe`. Catches bad enum values, missing required fields, and unknown
  fields (definitions are closed). `--no-validate` bypasses; `--schema` still routes
  to the on-disk cascade validator for arbitrary user schemas.
- **Docs.** `prime`/`guide` updated to draw the three write doors (assert →
  `facts add`/`observe`; import → `import`; mu bridge → `pudl mu …`), document
  `facts promote` and `feedback`, and fix all moved-command references.

## Deferred (later phases)

- Phases B (datalog aggregation), C (decay view), D (FTS) per the plan.
