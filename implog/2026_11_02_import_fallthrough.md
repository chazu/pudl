# Import fall-through: infra types removed, dry-run explains, built-ins refresh

Without `--schema`, an import never fails for lack of a type: a record that
matches nothing is stored as the catch-all `pudl/core.#Item`, and a record
with `name` (and `kind`) matched the generic built-ins `pudl/infra.#Organization`
or `pudl/infra.#Service`, which hid it as a "match". `--dry-run --explain`
printed no trace, so there was no way to see why.

## Changes

**`infra.#Organization` and `infra.#Service` removed** from
`pudl/infra/infra.cue` and from the `pudl/catalog` registry. `#Account`,
`#Platform`, `#ServiceBinding` and `#Environment` remain.

**Built-ins refresh in existing workspaces.** Bootstrap files only reached an
existing workspace when one was missing (or on `pudl init`), so a changed
built-in, such as the trimmed `infra.cue`, never did. New
`internal/importer/bootstrap_stamp.go`: `BootstrapDigest()` hashes the
embedded built-ins, and `copyBootstrapSchemasTo` writes it to
`pudl/.bootstrap-digest`. `ensureBasicSchemas`, run by every importer,
rewrites the built-ins when the stamp differs. pudl owns these files (`pudl
init` already overwrote them), so this changes when the refresh happens, not
what it does. Retired files are still removed by `retireBootstrapFiles`.

**Doctor re-infers entries whose schema is gone.** An inferred entry assigned
a schema that is no longer loaded was `invalid — schema ... is not loaded`,
and nothing repaired it. Doctor now runs inference for it and reports
`mismatch`, so `pudl doctor --fix` (the inference repair) reassigns it.
Explicit assignments stay `invalid`, as before (`--include-explicit` re-infers
them).

**`--dry-run --explain` prints the trace.** `Preview` computed each record's
classification trace (for ambiguity) and dropped it. It now keeps it, as an
import does: the document's trace, or up to 32 collection items
(`recordExplanation`), which `displayImportExplanation` prints.

**Guidance.** `pudl guide observe` and `pudl guide agents`, the pudl-core
skill and `pudl prime` tell users and agents to check every new source with
`pudl import FILE --dry-run --explain`, and to fix records that land on
`pudl/core.#Item` or an unexpected type before importing. The dry-run
ambiguity hint now suggests `@pudl(match)`. `docs/workspace.md` describes the
automatic refresh.

## Public API

`importer.BootstrapDigest() string` (new). No other exported changes.

## Tests

- `TestEnsureBasicSchemasRefreshesStaleBuiltins`: a workspace with an old
  stamp and a stale `infra.cue` is refreshed by the next importer.
- `TestDoctorReinfersEntriesWhoseSchemaWasRemoved`: an inferred entry on
  `pudl/infra.#Service` is a mismatch, and the inference repair reassigns it.
- `TestImportDryRunExplainPrintsTrace`: traces for a document and for each
  collection item, and nothing written.

## Validation

`go test ./...`, `make test-race`, `make lint`, `make test-git-walkthrough`,
`make check-docs check-skills`: pass. By hand: a workspace created by the
previous build with a record classified `pudl/infra.#Organization` was
upgraded by its first import with the new binary (`#Organization` and
`#Service` gone from its `infra.cue`, digest written). Doctor then reported the
entry as a mismatch inferring `pudl/core.#Item`; `pudl doctor --fix` reassigned
it, and doctor passed.
