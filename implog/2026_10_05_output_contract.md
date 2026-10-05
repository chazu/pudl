# Output contract: injectable streams, clean --json, CLI papercuts

**Date:** 2026-10-05
**Branch:** `improve/output`

## Problem

`pudl import --json` printed an emoji banner and no JSON. cmd/ had 575
`fmt.Print*` calls, 9 direct `os.Stdout` writes and no use of cobra's
configurable output, so command output could not be captured in-process, and
warnings were mixed into results on stdout. Seventeen commands reported errors
through `CLIErrorHandler(true)`, which called `os.Exit` inside the command.
Several commands ignored `--json`, `snapshot list --json` printed `null`, `list
--limit` had no effect under the default page size, and `import --path DIR`
failed.

## What changed

### Streams

- `cmd/output.go`: `outw()` is the result stream (`rootCmd.OutOrStdout()`),
  `errw()` the diagnostic stream (`rootCmd.ErrOrStderr()`), `printJSON(v)`
  writes one indented JSON document on `outw()`.
- Every `fmt.Print*` and `os.Stdout` write in cmd/ goes through `outw()`,
  except `cmd/query*.go` and `cmd/rule*.go` (owned by the datalog work; see
  Remaining) and the interactive editor in `schema edit`, which keeps the real
  terminal. Direct `os.Stderr` writes go through `errw()`.
- Warning, note and diagnostic lines (`warning: could not record ...`,
  `note: removed ...`, the uninitialized-workspace warning, setup backup
  warnings) moved from stdout to stderr. Low-confidence markers and check
  advisories that are part of a human report stay on stdout.
- `internal/ui/streams.go`: `Streams{Out, Err}`, `FromCmd(*cobra.Command)`,
  `Warnf`, `Progressf`.
- `internal/ui/output.go`: `NewOutputWriterTo(w, format, pretty)`;
  `NewOutputWriter` keeps writing to stdout. `GetOutputWriter()` writes to
  `outw()`. `WriteJSON` encodes a nil top-level slice as `[]`.
- `internal/init` and `internal/repo`: `InitOptions.Out io.Writer` receives
  verbose output (default stdout); `pudl init` passes `outw()`.
- The CUE loader's verbose debug log goes to stderr.

### Errors without os.Exit

`pudlRunE(body)` adapts a command body to `RunE`. On failure it prints the
error through the new `errors.Display(w, err)` — the same message plus
suggestions `CLIErrorHandler` printed — on `errw()`, silences cobra's duplicate
error line and usage, and returns the error. `Execute` exits with
`exitCodeFor(err)`, which already maps a `PUDLError` to its own code, so exit
codes are unchanged. Converted: config, config set, config reset, delete,
export, import, list, module add/info/list/tidy, schema add, schema
status/commit/log, setup, show.

### JSON output

New `--json` support:

| Command | Shape |
|---|---|
| `import` | array, one entry per file: `ImportResult` fields + `status` (`imported`/`skipped`/`failed`) + `error` |
| `show` | `{entry, metadata?, raw?, raw_text?}`; stored JSON embedded byte-for-byte |
| `version` | `{version, commit, date}` |
| `config`, `config set`, `config reset` | effective configuration incl. `initialized` |
| `config --path` | `{config_file}` |
| `schema show` | `{schema, package, file, size_bytes, source}` |
| `model validate` | `{model, valid, problems}` (still exits non-zero when invalid) |
| `mu ingest-observe` | `{records, snapshot_id, origin}` |
| `mu ingest-manifest` | `{run_id, skipped, statuses_repaired, actions, cached, failed}` |
| `snapshot retain` | `{snapshot_id, retained}` |
| `facts retract` / `facts invalidate` | `{id, retracted}` / `{id, invalidated, latest}` |

Import progress (`Importing N files`, per-file lines) goes to stderr. A single
failed import under `--json` still prints its `failed` entry, then fails.

### Papercuts

- `list --limit N` caps the total results across all pages (default 0: no
  cap). JSON gains `total_matched` (matches before the cap); text output notes
  `[--limit N of M matching]`.
- `import --path DIR` imports the supported data files (`.json .ndjson .jsonl
  .yaml .yml .csv`, plus `.gz`/`.zst` variants) directly in DIR, in lexical
  order; `--recursive` descends. Hidden files and directories are skipped, so a
  workspace's `.pudl/` is never re-imported. `importer.IsImportableName`.

## Public API

- `cmd`: `outw`, `errw`, `printJSON`, `pudlRunE` (package-internal).
- `internal/ui`: `Streams`, `FromCmd`, `Streams.Warnf`, `Streams.Progressf`,
  `NewOutputWriterTo`.
- `internal/errors`: `Display(w io.Writer, err error)`.
- `internal/importer`: `IsImportableName(name string) bool`.
- `internal/init.InitOptions.Out`, `internal/repo.InitOptions.Out`.
- `internal/lister`: `ListResults.TotalMatched`; `ui.ListOutput.TotalMatched`.

## Tests

- `cmd/cli_harness_test.go`: `runCLI(t, args...)` runs the root command
  in-process with captured stdout/stderr, resetting every flag between runs;
  `cliWorkspace(t)` creates an isolated repository workspace and HOME.
- `cmd/cli_json_contract_test.go`: seeds a workspace as the Git walkthrough
  does and requires each JSON-capable command's `--json` stdout to be one JSON
  document (not `null`). A second test fails when a runnable command is
  neither exercised nor listed in `jsonContractExempt` with a reason.
- `cmd/import_paths_test.go`: directory expansion, import `--json` outcomes,
  `list --limit`.
- `cmd/output_test.go`: errors reported once with suggestions and the right
  exit code, no usage on runtime failure; diagnostics stay off stdout.
- `internal/lister/limit_test.go`: page-size arithmetic under a limit.

## Counts (cmd/, non-test)

| | before | after |
|---|---|---|
| `fmt.Print*` | 575 | 18 (all in `query*.go`, `rule*.go`) |
| `os.Stdout` writes | 9 | 1 (`schema edit` editor passthrough) |
| `os.Exit` | 19 call paths (17 via `CLIErrorHandler(true)` + root + guide) | 2 (`root.go` Execute, `guide` unknown topic) |

## Remaining

- `cmd/query.go`, `cmd/query_helpers.go`, `cmd/rule.go`, `cmd/rule_new.go`
  still print with `fmt.Print*` to the process stdout, so in-process callers
  cannot capture `pudl query` or `pudl rule` output. Left alone because the
  datalog work owns those files; converting them is the same mechanical change.
- Text-only under `--json` (exempt in the contract test, with reasons): guide,
  prime, setup, the schema Git wrappers, the module wrappers, schema
  add/migrate/reinfer, migrate identity, reclassify, and model scaffolding.
- `guide` exits 2 on an unknown topic from inside the command.
- `docs/cli-reference.md` is not updated here; it is due to be generated from
  the command tree.
