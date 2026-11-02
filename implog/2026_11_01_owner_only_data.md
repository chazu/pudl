# Owner-only raw data and catalog artifacts — pudl-oa4

Catalog data can hold a secret that redaction missed, yet raw payloads,
metadata, collection items, observe snapshots and the SQLite catalog were
written 0644 under 0755 directories, readable by every local user. They are
now written 0600 under 0700 directories, and `pudl doctor` reports and
tightens data written before the change.

## Boundary

Owner-only: everything under the workspace's `data/` tree (`.pudl/data` in a
repository workspace, `~/.pudl/data` in global mode) — `raw/` and its date
directories, `metadata/`, `sqlite/` (catalog, `-wal`, `-shm`, locks), `tmp/`
(staging, spools, publication journals) — plus bundle staging and restore.

Unchanged: files users share or edit — CUE schemas, rules, models,
definitions, populators, `workspace.cue`, `config.yaml`, `.gitignore`,
`.gitkeep`, README, skills and generated docs. They are meant to be read,
reviewed and committed, and they are not evidence. The doctor scan covers
`<root>/data`, where the catalog lives. A repository workspace's `data_path`
must be `<root>/data` (`cmd/workspace_paths.go`); a global workspace whose
`data_path` was moved elsewhere gets owner-only new writes there, but the scan
does not visit that directory.

Already private and left alone: `os.CreateTemp`/`os.MkdirTemp` call sites
(import staging, decompression, transform spools, stdin, export spools and
output, bundle output), `internal/filelock` (0600; it cannot import
`artifacts`, which imports it), and `cmd/run_command_populate.go`, whose
output file lives in a private `MkdirTemp` directory (that file is being
replaced by `pudl check`).

## Modes in one place: `internal/artifacts/perms.go`

`internal/artifacts` already owned publication of catalog files, so the modes
live there:

```go
const (
	PrivateFileMode fs.FileMode = 0o600
	PrivateDirMode  fs.FileMode = 0o700
)
func WriteFilePrivate(path string, data []byte) error
func CreatePrivate(path string) (*os.File, error)   // O_RDWR|O_CREATE|O_TRUNC
func MkdirPrivate(dir string) error                 // MkdirAll, 0700
func CreateFileExclPrivate(path string) error       // empty 0600 file unless one exists

type LoosePath struct { Path string; Mode fs.FileMode }
func FindLoosePaths(dataDir string) ([]LoosePath, error)
func (p LoosePath) Tightened() fs.FileMode
func Tighten(p LoosePath) error
```

Like `os.WriteFile` and `os.MkdirAll`, the helpers leave existing files and
directories' modes alone; the doctor repair handles those.

## Call sites changed

| File | Before | After |
|---|---|---|
| `internal/importer/staging.go` (raw date dir, twice) | `MkdirAll 0o755` | `MkdirPrivate` |
| `internal/importer/metadata.go` (`.meta` files) | `WriteFile 0644` | `WriteFilePrivate` |
| `internal/importer/enhanced_importer.go` (`metadata/`, `tmp/`) | `MkdirAll 0755/0o755` | `MkdirPrivate` |
| `internal/importer/collection_stream.go` (item files, entries spool) | `WriteFile 0o644`, `os.Create` | `WriteFilePrivate`, `CreatePrivate` |
| `internal/observe/ingest.go` (snapshot file) | `WriteFile 0644` | `WriteFilePrivate` |
| `internal/observe/prepare.go` (`tmp/`, spool, record files) | `MkdirAll 0o755`, `os.Create`, `WriteFile 0o600` | `MkdirPrivate`, `CreatePrivate`, `WriteFilePrivate` |
| `internal/artifacts/publish.go` (published `raw/`/`metadata/` dirs) | `MkdirAll 0o755` | `MkdirPrivate` |
| `internal/artifacts/lock.go` (`data/sqlite` for `artifacts.lock`) | `MkdirAll 0o755` | `MkdirPrivate` |
| `internal/database/catalog.go` (`data/sqlite`, `catalog.db`) | `MkdirAll 0755`, SQLite-created file | `MkdirPrivate`, `CreateFileExclPrivate` before `sql.Open` |
| `internal/init/init.go` (global `data/`) | `MkdirAll 0755` | `MkdirPrivate` |
| `internal/repo/init.go` (`data/`, `raw/`, `metadata/`, `sqlite/`) | `MkdirAll 0o755` | `MkdirPrivate` |
| `internal/bundle/bundle.go` (six sites, already private) | `0o700`/`0o600` literals | constants and helpers |

**SQLite.** modernc's SQLite (transpiled from C) creates a new database with
`SQLITE_DEFAULT_FILE_PERMISSIONS` (0644, less umask) and gives the `-wal`,
`-journal` and `-shm` files the database file's mode. Pre-creating an empty
`catalog.db` at 0600 (an empty file is a valid empty database) under the
existing init lock therefore makes all three owner-only without touching the
process umask, which would race with other goroutines' file creation. An
existing catalog keeps its mode until the repair below tightens it; its `-shm`
is recreated with the database's mode.

## Doctor: `data-permissions` repair

`internal/repair/data_permissions.go` registers `data-permissions` (last in
`Registry`; it depends on no other repair). Its Diagnose is the doctor check:
plain `pudl doctor` lists every regular file or directory under `<root>/data`
with group or other bits set as a change (`From: 0644`, `To: 0600`), with
status `needed` and the preview/apply argv. It is a repair rather than a
separate health check so the condition is reported once, and because a
failing health check would block `--fix` itself.

- `pudl doctor --fix --dry-run --only data-permissions` previews.
- `pudl doctor --fix --only data-permissions` chmods each path to its owner
  bits (`mode & 0700`); per-path failures make the status `partial`.
- Symlinks are not followed or reported; paths removed mid-scan are skipped.
- Like every repair, it reports `no catalog` when there is no catalog.

`doctor --help` lists the repair; `docs/cli-reference.md` and
`cmd/testdata/help.golden.json` are regenerated.

## Tests

- `internal/database/catalog_perms_test.go`: a new catalog's `sqlite/` is
  0700 and `catalog.db`, `-wal`, `-shm` are 0600.
- `internal/importer/permissions_test.go`: an NDJSON collection and a single
  JSON import produce 0600 payload, item and metadata files in 0700
  directories; nothing under `raw/`, `metadata/`, `sqlite/`, `tmp/` is loose.
- `internal/observe/permissions_test.go`: an observation's record and
  snapshot files are 0600, and nothing under the data directory is loose.
- `cmd/doctor_permissions_test.go`: doctor flags a pre-existing 0644 payload
  (`needed`, `0644 -> 0600`); `--fix --dry-run` leaves it; `--fix --only
  data-permissions` makes it 0600 and its directory 0700 (`applied`); a
  further run reports `ok`.

`CGO_ENABLED=0 go test ./...` passes; `CGO_ENABLED=0 mise exec -- make lint`
reports 0 issues.

## Line delta against `sre-focus`

Production Go: +191 −26 (net +165), of which 96 are the new
`internal/artifacts/perms.go` and 61 the new repair. Tests: +168 −0.
