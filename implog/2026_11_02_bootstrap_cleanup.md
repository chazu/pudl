# Retire pudl/brick; move the observe schemas out of pudl/mu

## Changes

**`pudl/brick` removed.** It held mu build-target schemas (`#Target`,
`#Interface`, `#Kit`), unused since the mu integration was removed on
2026-11-01. Nothing in production code, content packs or current docs used
it. Its three types were inference candidates for every record with a `name`.
The mu/brick inference guard test (`TestMuTargetVsBrickTarget`) went with it.

**Retired bootstrap files are removed from existing workspaces.**
`retireMemorySchema` became a table, `retiredBootstrapFiles`, mapping each
retired file to the SHA-256 of every version pudl shipped
(`pudl/nous/nous.cue`, and both shipped versions of `pudl/brick/brick.cue`).
An unmodified copy is deleted, along with its directory once empty; an edited
copy or a symlink is kept. Retirement now runs in `ensureBasicSchemas`, at the
start of every importer, as well as during a full bootstrap copy. Without it,
a stale `brick` package would keep loading and doctor would warn that it is a
user schema in the reserved `pudl/` namespace.

**`#ObserveResult` and `#ObserveSnapshot` moved to `pudl/core`**, in the new
file `pudl/core/observe.cue`. They are the live schemas observation ingest
writes: records that name no schema, and each snapshot collection. Their
resource types are now `pudl.observe` and `pudl.observe_snapshot`.
`pudl/mu` now holds only the legacy definitions kept so pre-removal catalog
entries still validate.

- `schemaname.Normalize` maps the old names (`pudl/mu.#ObserveResult`,
  `pudl/mu.#ObserveSnapshot`) to the new ones (`movedSchemas`), so stored
  entries resolve and doctor's schema-names repair rewrites them.
- The new file makes `ensureBasicSchemas` re-copy the built-ins in an
  existing workspace on its next import or observation. That refresh rewrites
  `mu.cue` without the moved definitions and retires `brick`, so no manual
  `pudl init` is needed.
- Resource IDs of new generic records and snapshots are namespaced by the new
  names. Neither carries history across entries: generic observe records are
  content-identified, and every snapshot is unique per run.

## Public API

No exported API changes. Unexported: `retiredBootstrapFiles`,
`retireBootstrapFiles`, `retireFile` (replacing `retireMemorySchema`);
`schemaname.movedSchemas`.

## Audit of the other bootstrap packages

The rest of `pudl/` was audited for this change; nothing else was removed.

- Live: `core`, `rules` (`convergence.cue` holds the live cross-model
  dependency rules under an old name), `aws` (imported by the AWS pack), `git`
  (the git-inventory pack), `dlktk` (the dialectic toolkit relies on pudl
  shipping it), and `mu` (legacy definitions, kept by design).
- No users found: `catalog`, `component`, `fs`, `registry`, `version`,
  `artifact`, `infra`, `k8s`, `linux`. `infra.#Service` also competes with
  real types in inference. Removing these changes what generic data
  classifies as, so it is left for a decision.

## Tests

- `retired_schemas_test.go`: `retireFile` removes only shipped versions and
  an emptied directory; `brick` is retired and no longer a built-in package.
- `schemaname_test.go`: the moved names normalize to `pudl/core`.
- Observe, database, scrub, inventory and bundle tests use the new snapshot
  name.

## Validation

`go test ./...`, `make lint`, `make test-race`, `make test-git-walkthrough`,
`make check-docs check-skills`: pass (lint 0 issues). By hand: a workspace created by the
previous build (with `brick` and the old `mu.cue`) was upgraded by its first
import with the new binary. `brick` was removed, `core/observe.cue` installed,
`#ObserveResult` gone from `mu.cue`, and `pudl doctor` exited 0 with a clean
reserved-namespace check.
