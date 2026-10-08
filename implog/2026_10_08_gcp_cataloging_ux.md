# Cataloging workflow simplification (GCP findings)

Source: `docs/gcp-cataloging-workflow-findings.md`, an agent's report from
cataloging GCP resources. Its claims were verified against the code. Proposals
went into `docs/design/2026-10-08-gcp-cataloging-ux.md`, which two adversarial
reviews (integrity, UX) revised substantially before implementation (review
log §10, deviations §11). User guide: `docs/projection.md`.

## Public API

### CLI

- **`pudl import [paths…]`** gains:
  - positional paths, which win over piped stdin
  - `--set path=value` (repeatable, overwrites, JSON-typed values)
  - `--dry-run` (status `dry-run`, findings in a `preview` object)
- **Import output** gains these fields under `--json`, each with a summary
  note:
  - `identity_unresolved`, `identity_error`
  - `redacted`, `reassigned`
  - `facts`, `fact_warnings`
- **`pudl import --schema`** on already-cataloged content moves entries that
  satisfy it to that schema, re-sequencing versions when `resource_id`
  changes.
- **`pudl facts reproject [--dry-run]`** syncs schema-projected facts.
- **`pudl query`:**
  - `true`/`false` constraints are typed
  - stderr warns when projection is stale or broken
  - lint for the rules the query depends on
- **`pudl list` / `export` / `schema reinfer --schema`:** a value containing
  `#` matches whole definition names (`#Route` no longer matches `#Router`).
  LIKE wildcards are escaped. When an anchored match finds nothing, `list`
  hints at the fragment.
- **`pudl model new --populate`** accepts `command:<cmdline>`, split
  shell-style, no shell. `plugin:<name>` now emits a `plugins:` block from mu's
  cache.
- **`pudl run --populate command:<cmdline>`** runs a command ad hoc.
- **`pudl model new --input`** values parse exactly (no float64 rounding).

### Schema (`_pudl` and `#SystemModel`)

- **Field paths** (identity, facts, sensitive fields, `--set`): quoted segments
  for dotted keys and `[*]` wildcards.
- **`_pudl.facts`:** `{rel: {each?, args: {name: path | {path, default?,
  trim_prefix?} | {exists}}}}`. Requires `identity_fields`. Implicit args are
  `entry_id` and `resource_id`.
- **`_pudl.sensitive_fields`:** redacted to `"[REDACTED]"`, fails closed. Any
  `sensitive_mode` key is an error.
- **`#CommandObserve {runs: [#CommandRun], schema?, timeout?}`** and
  **`#CommandRun {argv, set?, dir?}`**.
- **`populate` is optional:** a model without it is checks-only, with no
  `desired` or `converge`.

### Go

- **`internal/fieldpath`:** `Parse`, `MustParse`, `FromKeys`, `Lookup`,
  `LookupAll`, `Set`, `Replace`, `Overlaps`, `HasWildcard`.
- **`internal/redact`:** `Registry` (`NewRegistry`, `Any`, `For`), `Apply`,
  `Describe`, `Placeholder`.
- **`internal/projection`:**
  - spec types: `ParseRelations`, `RelationSpec`, `ArgSpec`
  - `Registry` (`For`, `Schemas`, `RelationArgs`), `SchemaSpec`
  - computing facts: `Compute`, `Prepare`, `Prepared`, `Observe`
  - keeping them current: `Sync`, `SyncReport`, `Stale`
  - reporting: `Lint`, `Tally`
- **`database`:**
  - `projection_state` table (migration 23) and an index on
    `current_facts(source)`
  - `CatalogTx.ReconcileProjection` (observation invalidates, correction
    retracts; re-wanted facts reappear)
  - projection-state accessors, `ProjectionOrphans`,
    `EntriesNeedingProjection`
  - `CatalogTx.ReassignEntry`
  - `SchemaFilterCondition`, `EscapeLike`
  - `SnapshotSourceCommand`
  - the `projection:` source prefix is reserved for general fact writers
- **`importer`:**
  - `ImportOptions.{Set, DryRun}`, `FieldAssignment`
  - `EnhancedImporter.Preview`, `PreviewReport`
  - a transform pre-pass, with schema assignments spooled to disk and the
    original bytes kept when nothing changes
- **`mubridge.ObserveIngest.{ManualSchema, Chain, Redactor, Projection}`**;
  the result gains `Redacted`, `Facts`, `FactWarnings`.
- **`inference.InferenceTrace.Alternatives`:** same-score matches from other
  inheritance families, computed only when tracing.
- **`validator.SchemaMetadata`:** strict `SensitiveFields/SensitiveError`,
  `FactsSpec/FactsError`.
- **`systemmodel`:**
  - `Populate.{Runs, Schema, Timeout, Absent}`, `CommandRun`
  - `KindCommand`, `KindNone`

## Behaviour worth knowing

- **Facts follow the latest observation.** A resource's facts follow its most
  recently *observed* entry, including dedup hits and whole-file re-imports, so
  a resource that changes and then reverts reports its current state. Sync runs
  after import, before run checks, and after reinfer, delete, snapshot prune
  and identity migrate.
- **Broken specs.** A broken facts spec disables projection for its schema
  without emptying existing facts, and every surface reports it.

## Verification

- `CGO_ENABLED=0 go test ./...` and `go vet ./...` pass.
- `go test -race` passes on importer, projection, mubridge, database and cmd.
- **New tests:**
  - field paths and quoted identity
  - CLI value typing
  - the anchored schema filter
  - `--set` with exact numbers and dedup
  - redaction (import, observe, fail-closed, unchanged-source hash)
  - reassignment
  - dry run
  - ambiguity
  - reconcile revert
  - projection follows the latest observation (import and observe)
  - Sync bootstrap, reproject, broken spec and orphan
  - spec validation
  - the end-to-end import → reproject → query → lint
  - command populate fan-out with checks, and checks-only models
  - command-line splitting
- **Manual smoke test** in a scratch workspace: dry-run → import `--set` →
  query over projected facts → `model new --populate command:` → run with a
  failing check.

## Follow-up: `pudl guide` topics

- **import:** positional paths, the `--dry-run` → `--set` workflow,
  reassignment on re-import, and unresolved-identity reporting. The
  `logs/**/*.json` example was wrong (Go's `filepath.Glob` has no `**`) and is
  replaced with a `--recursive` directory example.
- **schemas:** the `_pudl` block (quoted identity paths, `facts`,
  `sensitive_fields`) and anchored `--schema` filters.
- **facts:** projected facts and `facts reproject`.
- **datalog:** querying imported fields, the `entry_id` join, typed CLI
  constraints, and the rule lint.
- **models:** the command populate arm with a comprehension fan-out,
  checks-only models, and the `command:` scaffold and ad-hoc forms.
- **mu:** command populate needs no mu.
- **agents:** dry-run first, `export` for exact payloads, `total_matched`, and
  `_pudl.facts` instead of post-processing payloads.
- **troubleshooting:** empty query/check results, and an empty
  `list --schema`.

## Follow-up: pudl-core skill

The pudl-core skill source is `skills/pudl-core/SKILL.md`. The
`.claude/skills/pudl-core` symlink points at it, and the embedded copy is
regenerated with `go generate ./internal/skills`. Changes:

- **Scaffolds:** the `command:` forms for both scaffold and ad-hoc run.
- **Import:** positional paths, `--dry-run`, `--set` and reassignment on
  re-import.
- **list and export:** anchored `--schema`, `total_matched`, and `export` for
  exact payloads.
- **Facts:** `facts reproject`, plus a new "Querying imported data" section
  covering `_pudl.facts`, `sensitive_fields`, the `entry_id` join, typed
  constraints and the lint warnings.
- **Populate arms:** the command arm and checks-only models.
