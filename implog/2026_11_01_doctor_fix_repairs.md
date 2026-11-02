# Repair commands folded into `pudl doctor --fix` (pudl-4fx.2)

`pudl doctor` now diagnoses every catalog condition that a standalone repair
command used to fix, names the repair, and `pudl doctor --fix` applies it. The
six standalone commands are deleted without aliases; replacements are listed in
`docs/retired-commands.md#catalog-repairs`.

| Repair (`--only`) | Condition diagnosed | Former command |
| --- | --- | --- |
| `schema-names` | schema names not in canonical `<package>.#<Definition>` form | `pudl schema migrate` |
| `schema-refs` | unresolved item schema references now present in the schema cache | `pudl reclassify [--ref]` |
| `inference` | inferred entries whose inference now selects another schema | `pudl schema reinfer` |
| `identity` | entries with no `resource_id`, or one still namespaced by the leaf schema | `pudl migrate identity [--recompute]` (and the `pudl migrate` group) |
| `projections` | projected facts out of line with the catalog or the facts specs | `pudl facts reproject` |

Repairs run in that order, so names are canonical before classification,
classification settles before identity, and projections follow the final
entries.

## Default: `--fix` applies, `--fix --dry-run` previews

Plain `pudl doctor` never writes and already shows what every repair would
change, so it is the preview. `--fix` is the explicit mutation intent, and
applies. `--fix --dry-run` previews through the same apply path (for the
projections repair this is the real `projection.Sync` dry run). This matches
the former commands (`facts reproject`, `migrate identity`, `schema migrate` and
`snapshot prune` applied by default with `--dry-run` to preview) and the
repository's agent-first output contract: the interactive confirmation prompt
of `schema reinfer` is gone because `--fix` already confirms, and a prompt
cannot be answered from `--json` automation. `--dry-run` without `--fix` is an
error. `--fix` refuses to write when a workspace health check failed.

## Public API

`internal/repair` (new):

- `type Repair struct { Name, Condition string; Diagnose func(ctx, *Env) (Result, error); Apply func(ctx, *Env, dryRun bool) (Result, error) }`
- `func Registry() []Repair`, `func Names() []string`, `func Select(only []string) ([]Repair, error)`
- `type Env struct { DB; Inferrer; Projections; SchemaCacheRoot; Options }`
- `type Options struct { SchemaRef; DriftEntryIDs; DriftScanned; IncludeExplicit; EntryID; Schema; Origin }`
- `type Result struct { Name, Condition, Status, Message; Pending, Changed, Failed int; Changes []Change; Errors []string; Details any }`
- `type Change struct { Subject, From, To string }`
- `func Run(ctx, Repair, *Env, apply, dryRun bool) Result`, statuses
  `ok|needed|applied|partial|error|skipped`
- `func LoadPayload(database.CatalogEntry) (any, error)`

Adding a repair (for example the planned policy-leaf move, `pudl-xf5.8.1`) is
one file defining a `Repair` value and one entry in `Registry()`.

Other additions:

- `database.(*CatalogDB).PendingSchemaNameMigrations() ([]SchemaNameChange, error)`;
  `MigrateSchemaNames()` now returns the `[]SchemaNameChange` it applied.
- `importer.DecodeStoredFile(storedPath, format)` (the former
  `cmd.loadReinferData`).

## CLI

New `pudl doctor` flags: `--fix`, `--dry-run`, `--only NAME[,NAME]`,
`--schema`, `--origin` (scope the catalog scan and the inference repair),
`--include-explicit` (re-infer every entry in scope, the former `reinfer
--all/--entry/--schema/--origin`), `--schema-ref` (the former `reclassify
--ref`).

JSON is `report_version: 2` with `mode` (`diagnose|dry-run|fix`); each
`repairs` item is a `Result` plus `preview_argv`/`apply_argv` when needed.
Version 1 (no `report_version`) carried only `reason/preview_argv/apply_argv`.
After `--fix` writes, the catalog is re-scanned so `catalog` and `ok` describe
the repaired state; an `error` or `partial` repair fails the command. A repair
that changes entries also resyncs projected facts unless `projections` ran, as
the former commands did.

## Behavior notes

- Inference drift is taken from doctor's own catalog scan, so the inference
  repair never re-infers explicit producer assignments unless
  `--include-explicit` is given. It does not repeat inference for the whole
  catalog.
- Stale identities are detected exactly: the stored `resource_id` equals the
  leaf-namespaced id and differs from the root-namespaced one. Only those
  entries are recomputed, and only the resources they join are re-sequenced.
  The former `--recompute` rewrote every entry; it is not kept, because
  diagnosis now finds the entries it was for and a blanket rewrite would
  renumber unrelated histories.
- The identity repair decodes payloads with the import decoder
  (`importer.DecodeStoredFile`) instead of `json.Unmarshal`, so YAML, CSV and
  NDJSON entries and large JSON integers produce the same identity values as
  the import did.
- Diagnosis does not create the schema cache directory.
- Messages that said `run: pudl facts reproject` now name
  `pudl doctor --fix --only projections`.

## Tests

- `internal/repair/identity_test.go`: version assignment; backfill, leaf
  recompute with version merge, catchall without payload, dry run, and clean
  re-diagnosis.
- `cmd/doctor_repairs_test.go`: versioned diagnose report that writes nothing;
  flag validation; each former command through doctor (schema names with dry
  run, inference drift and `--include-explicit --entry`, identity backfill,
  schema refs with `--schema-ref` and no cache creation).
- `cmd/projection_e2e_test.go` and `cmd/cli_json_contract_test.go` use
  `doctor --fix [--dry-run]` for reprojection.

## Line delta (against 6925bfe)

Production Go +1038 / -1096 (net -58). Test Go +374 / -184 (net +190).
Docs and generated files +186 / -432 (net -246).

## Follow-ups

- `schema-refs` exists only for typed-envelope imports and the mu schema cache
  (`internal/muschemas`). The convergence/mu removal deletes both; it should
  delete `internal/repair/schema_refs.go`, its `Registry()` entry, the
  `--schema-ref` flag, `SchemaCacheRoot` and the matching test.
