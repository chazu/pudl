# Repository-local run-set kick-the-tires

> **Resolved later on 2026-08-05:** the mu plan projection and PUDL observation
> phase boundary were fixed; the sealed producer/consumer run-set now passes
> through exact approval/resume. Earlier blocker text below is historical.

## Scope

Created and executed a hermetic operator checklist against the PUDL repository's
own `.pudl` workspace. The fixture uses a local CUE schema/model family and a
small NDJSON mu plugin/fake secret provider. Source fixtures live under
`.pudl/schema` and `.pudl/populators`; generated binaries, reports, sentinels,
provider state, and command transcripts live under the ignored
`.pudl/data/kick-tires` tree.

## Public behavior fixed

- `pudl model validate <name>` now validates the retained authoring template.
  A structurally valid model with unresolved plain bindings succeeds; its final
  concrete values remain validated during `pudl run`.
- Resolving a requested malformed model now returns its template error (for
  example, invalid RFC 6901 syntax or a missing binding annotation) instead of
  incorrectly reporting that the model was not found.
- `pudl repo init` now creates and tracks `.pudl/populators/`; `pudl doctor`
  recognizes that public authoring directory.
- A pending single-model convergence approval now persists `--mu-root` and
  restores it in `pudl run resume`, so resume reaches the same mu project in a
  new process.

## Executed matrix

- Real mu producer-to-consumer observation in reverse CLI order, proving
  producer-first graph order, exact current-run snapshot binding, consumer
  config injection, scalar digest/provenance, and standalone snapshot reuse.
- Missing producer, cycle, invalid pointer, unannotated consumer slot,
  unauthorized source projection, and failed-producer/no-history cases with
  plugin sentinels and durable structured reports.
- Non-sealed run-set approval, process-separated resume, stale-plan rejection,
  explicit rejection, single-model sealed provider write/resolution, and
  workspace write-policy denial.
- Lost-receipt/apply/observe recovery tests, stale workspace cleanup, approval
  races, two simultaneous real run-sets, final SQLite integrity, and global
  catalog byte-identity.

## Unresolved live integration gap

The real sealed run-set path fails closed before approval. PUDL renders target
sealed refs, and the local plugin receives and forwards them, but mu v0.3.3's
`build --plan --json` projection omits `sealed_inputs`, `sealed_input_modes`,
`sealed_outputs`, and `sealed_output_modes`. PUDL therefore cannot prove the
strict action claims required by `annotateSealedActionClaims`. Existing
fake-runner tests synthesize those fields and do not establish real bridge
compatibility. This release blocker is tracked as `pudl-olm`; documentation and
CLI usage text now describe the fail-closed current state.

## Verification

The targeted regressions and live fixture passed after the fixes. The full
repository gate results and exact operator evidence are recorded in
`.pudl/data/kick-tires/results.md`.
