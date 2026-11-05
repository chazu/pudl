# Content packs leave examples/: `pudl pack`

## Decision

The AWS and GCP content packs are products, not examples. Their sources moved
from `examples/aws`, `examples/gcp` to `packs/aws`, `packs/gcp` (a pure
`git mv` commit, history kept). They install with a new `pudl pack` command.
`pudl example` keeps the tutorials (`git-inventory`, `gcp-network-hygiene`),
which stay in `examples/`.

Hard break, no migration: `pudl example install gcp|aws` now fails with the
ordinary `unknown example` error, and no alias exists. Pack installs are
recorded in `<workspace>/.packs/<name>.json`. An existing
`.examples/gcp.json`/`aws.json` record is not read or migrated, so those files
count as installed without a record: changed files are conflicts until one
`pudl pack install <name> --overwrite` (which keeps `<file>.bak`).

## Package layout

- `internal/fileset` (new; the installer moved here from `internal/examples`,
  history kept): installs a set of embedded files into a workspace, records
  digests, upgrades unedited files, reports conflicts, backs up on
  `--overwrite`. Generic over a `Kind` (CLI noun + record directory).
  (`internal/bundle` already exists for workspace bundles, so the installer
  was not put there.)
- `internal/examples`: tutorial registry (`Kind{example, .examples}`).
- `internal/packs` (new): pack registry (`Kind{pack, .packs}`), embedding
  `packs/gcp` and `packs/aws`.
- `cmd/fileset_install.go`: shared workspace resolution (`--global`) and the
  install/report flow for both commands; `cmd/example.go`, `cmd/pack.go` hold
  only their help and next-step hints.

Dependencies: `cmd` → `internal/{examples,packs}` → `internal/fileset`;
`internal/packs` → `packs/{gcp,aws}` (embed only). No cycles.

## Public CLI surface

```
pudl pack install <gcp|aws> [--overwrite] [--global] [--json]
    Install or upgrade a content pack. Same behaviour as example install:
    identical files untouched, unedited files upgraded, conflicts listed and
    nothing written unless --overwrite (keeps <file>.bak). --json returns
    {pack, root, files, added, upgraded, replaced, unchanged, backups}.
pudl pack list [--global] [--json]
    Lists packs with a state from the workspace's install record:
    installed | outdated | not-installed (no state outside a workspace).
    --json returns {root, packs: [{name, summary, state}]}.
pudl example install <git-inventory|gcp-network-hygiene> [--overwrite] [--global] [--json]
    Unchanged except it no longer knows gcp/aws. --json key stays "example".
```

`pack` is excluded from global auto-initialization, like `example`.

## Go API

```go
// package fileset
type Kind struct{ Noun, RecordDir string }
type Options struct{ Overwrite bool }
type Result struct{ Files, Added, Upgraded, Replaced, Unchanged []string; Backups map[string]string }
type ConflictError struct{ Kind Kind; Name string; Paths []string }
func Install(root string, kind Kind, name string, files map[string][]byte, opts Options) (*Result, error)
func TreeFiles(fsys fs.FS) (map[string][]byte, error)
type State string // NotInstalled, Installed, Outdated
func RecordedState(root string, kind Kind, name string, files map[string][]byte) (State, error)

// package packs
var Kind fileset.Kind
type Pack struct{ Name, Summary string }
func List() []Pack
func Names() []string
func Files(name string) (map[string][]byte, error)
func Install(root, name string, opts fileset.Options) (*fileset.Result, error)
func State(root, name string) (fileset.State, error)

// package examples
var Kind fileset.Kind
func Names() []string
func Files(name string) (map[string][]byte, error)
func Install(root, name string, opts fileset.Options) (*fileset.Result, error)
```

## Other changes

- Every test and smoke test that installed gcp/aws uses `pack install`;
  smoke `TestSmoke_ExampleInstallGlobal` became `TestSmoke_PackInstallGlobal`
  (Makefile `test-git-walkthrough` runs `PackInstall.*` too).
- Docs and hints: README, docs (README, concepts, getting-started, governance,
  cross-model-dependencies, schema-authoring, gcp-network-hygiene, frontend,
  TESTING, the current GCP CAI design doc), `cmd/guide/*.txt`, `pudl prime`,
  `skills/pudl-core` (+ embedded copy), pack READMEs and pack model comments,
  the web UI coverage notice (bundle rebuilt with `make web`).
- `docs/cli/notes/pack_install.md` (new), `example_install.md` trimmed;
  `docs/cli-reference.md` and `cmd/testdata/help.golden.json` regenerated.
- `docs/retired-commands.md`: "Content packs" section.
- `implog/` and `docs/plan.md` history left as written.

## Tests

- `internal/fileset/install_test.go`: idempotent install, upgrade, conflicts
  (edited / unrecorded), backups, `RecordedState` (synthetic file set).
- `internal/packs/packs_test.go`: records under `.packs`; an `.examples/gcp.json`
  record is ignored (conflict until `--overwrite`); unknown pack error.
- `internal/examples/sources_test.go`: records under `.examples`; gcp/aws unknown.
- `cmd/pack_test.go`: `pack list` states; `example install gcp|aws` fails.
- `cmd/cli_json_contract_test.go`: exercises `pack list`, `pack install`.
