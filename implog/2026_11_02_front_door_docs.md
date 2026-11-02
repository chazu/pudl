# Front-door documentation rewrite

The user-facing entry docs now describe pudl as it is after the SRE-focus
work: discover, catalog and govern infrastructure, never change it. They use
the shared docs brief's terms (observation, snapshot, scope, completeness,
entry, resource, type, policy, model, check, finding, waiver, report).
No code changed, and no public API changed.

## Files

| File | Before | After | Change |
| --- | ---: | ---: | --- |
| `README.md` | 240 | 183 | Rewritten: what pudl is, install, an offline five-minute tour on the GCP pack fixtures, a "how it fits together" diagram, features grouped by discover / catalog / govern / agents, doc links |
| `FEATURES.md` | 137 | removed | Folded into the README's feature list (see below) |
| `docs/getting-started.md` | 156 | 253 | Part 1 keeps the Git inventory walkthrough the smoke test runs, now explaining each concept as it appears; new Part 2 covers policies, checks, finding changes, owners and a waiver on the GCP fixtures |
| `docs/concepts.md` | 229 | 247 | Rewritten around the brief's mental model; value wiring, convergence, the import-pipeline internals and the stale layout removed |
| `docs/collections.md` | 142 | removed | Merged into `concepts.md` ("Collections"); typed envelopes dropped |
| `docs/workspace.md` | 64 | 83 | Rewritten: layout table, what to commit, definition resolution, vendored packages, migration from global definitions, bundles; approval records dropped |
| `docs/VISION.md` | 169 | 45 | Rewritten: purpose, principles, out of scope (changing infrastructure), direction |
| `docs/README.md` | 44 | 64 | Restructured: Start here, Guides, Reference, Content packs, Internals and history |
| Total | 1181 | 875 | |

`docs/retired-commands.md` gained two sections (+27 lines) holding content the
old README carried: "Consolidated commands" (the former-to-current command
table not already covered there) and "Agent memory application". The links in
`docs/plan.md` and `docs/design/2026-09-30-ux-simplification-report.md` that
pointed at the removed README anchors now point there.

## Decisions

- **FEATURES.md folded into the README.** It was already marked historical and
  listed removed commands. A second feature list duplicates the README and the
  CLI reference and goes stale; one short grouped list in the README replaces
  it. Nothing outside implogs linked to it.
- **collections.md merged into concepts.md.** Without typed envelopes it was a
  short note on NDJSON items and membership plus `pudl list` flags the CLI
  reference covers.
- **workspace.md kept separate.** `cmd/config_paths.go` prints
  `docs/workspace.md` as the migration guide.
- **README tour on the GCP fixtures.** It runs offline. Fixture records are
  content-addressed, so the failing firewall is `vomap-foqab` in every
  workspace and the tour can name it.

## Verification

- A throwaway script checked every `pudl` command path and `--flag` in the
  rewritten files against `go run . help --json`. The only remaining mismatch
  is the deliberate mention of the retired `pudl run` that links to the
  retired-commands page. Every relative link and anchor in the rewritten files
  resolves.
- The README tour and both parts of the tutorial were run in temporary
  workspaces with an empty home directory.
- `CGO_ENABLED=0 go test ./...`, `make check-docs check-skills
  test-git-walkthrough` and `make lint` pass.
