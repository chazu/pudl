# 2026-06-30 — Rename catalog `definition` → `target`

The catalog `definition` column was a fossil of the removed World-A subsystem.
Mechanically it held the **mu target name** that produced the rows (`targetToDefinition`
just stripped `//`). That made `pudl status` confusing: a model, a model's populate
phase, and a bulk plugin observe all appeared as "definitions" (user observation:
"a model is a definition, a populate action is another definition, a collection of
resources is a definition"). Renamed to `target` — what it actually is.

## What changed (commit `576e0cd`)

- **DB**: `catalog_entries.definition` → `target`. Idempotent in-place migration
  `renameLegacyDefinitionColumn` (drop-view-first, like `dropLegacyMethodColumn`;
  preserves data). Index `idx_definition` → `idx_target`; `catalog_entry_edb` view
  column; fresh DBs add `target`.
- **Go (internal/database)**: `CatalogEntry.Definition` → `Target`; `DefinitionStatus`
  → `TargetStatus`; `GetDefinitionStatuses` → `GetTargetStatuses`; all SQL/scan/params.
- **mubridge**: `targetToDefinition` → `normalizeTarget`; ingest stores into `target`.
- **cmd**: `pudl status` header "Definition" → "Target"; `pudl status [target]`;
  `modelDefinition` → `modelTargetKey`; `lister` mirror field.
- **datalog**: the `catalog_entry` relation's queryable column `definition` → `target`
  (`docs/datalog.md`); `docs/cli-reference.md` `pudl status` section.
- **Test**: `TestRenameLegacyDefinitionColumn` verifies the rename preserves a seeded
  row's value (`target` survives, `definition` gone).

## Explicitly NOT renamed (the overloaded-word trap)

The CUE/schema-definition sense stays "definition":
- `#Definition`, schema def names; `schemaname`/`schema`/`schemagen`.
- The `pudl model list` **DEFINITION** column = the CUE def name (`models.#GithubChazu`)
  — a genuinely different concept from the catalog target. The two commands now read
  cleanly distinct: `status` → Target (`models/github-chazu`), `model list` →
  DEFINITION (`models.#GithubChazu`).
- `mubridge/envelope.go`'s schema `Definition` (`#EC2Instance`, `<module>@<v>#<def>`).
- The `--only` convergence-unit wording (a separate vocabulary axis the V1 spec owns).

## Verified

On the live catalog: column renamed `definition` → `target` in place, all rows
preserved (incl. the host-inventory and model-instance rows), `pudl status` reads from
`target`. Full `go test ./...` green.

## Side finding — `pudl init --force` clobbers the repo's SKILL.md

Running `pudl init --force` (to refresh the installed schema, finding D from the k8s
validation) overwrote the repo's maintained `skills/pudl-core/SKILL.md` with a **stale
embedded copy** (stripped the frontmatter + the "no execution layer" content). Reverted
it; kept out of this commit. `init --force` installing an old embedded skill over a
git-tracked, newer file is a real regression worth fixing separately (the embedded skill
asset is behind the repo's).
