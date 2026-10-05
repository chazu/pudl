# CI hardening: Go 1.26.6, golangci-lint, govulncheck

## Toolchain and dependencies

- Go raised from 1.26.2 to 1.26.6 in `go.mod`, `mise.toml`, the CI `GO_VERSION`,
  and the install/requirements text in README, `docs/getting-started.md`,
  `docs/architecture.md` and `docs/TESTING.md`. 1.26.6 fixes four reachable
  standard-library advisories (GO-2026-6218, -6091, -6090, -6088).
- Upgraded past module advisories: `golang.org/x/text` v0.34.0 → v0.39.0
  (GO-2026-5970, reachable), `golang.org/x/net` v0.50.0 → v0.56.0 (GO-2026-5026
  reachable; GO-2026-5942 module-level), `github.com/klauspost/compress`
  v1.18.3 → v1.18.7 (GO-2026-5841, module-level). `x/sys` and `x/sync` moved with
  them. `govulncheck` now reports no vulnerabilities.
- The `golang:1.26.2` string in the scaffolded GitLab example (`internal/init`)
  is sample CI data, not the toolchain, and is unchanged.

## Lint configuration

`.golangci.yml` (golangci-lint v2) enables only govet, staticcheck, errcheck,
ineffassign, unused, bodyclose and sqlclosecheck, plus the gofmt formatter, and
lints the `smoke` build tag too.

errcheck exclusions are configuration, each with its reason in the file:
terminal writes (`fmt.Fprint*`), `Tx.Rollback`, `Rows.Close`/`Stmt.Close`,
releasing read handles and pooled connections (`*os.File`, `*sql.DB`,
`*sql.Conn`, `CatalogDB`, `runCatalog`, `Lister`, `EnhancedImporter`,
`filelock.Lock.Release`), best-effort temp cleanup (`os.Remove`,
`os.RemoveAll`), and init-time cobra wiring. errcheck is not applied to
`_test.go` files (213 findings there, mostly test setup). Because `(*os.File).Close`
is excluded, every path that *writes* a file now checks `Close` explicitly.

## Findings fixed (first run: 1,128 issues)

- **errcheck — 1,035**: 641 terminal writes and the read-handle/cleanup idioms
  are covered by the configuration above; 213 were in tests. Real defects fixed:
  - `facts stats` ignored `rows.Scan` errors and never checked `rows.Err()`.
  - YAML export dropped `encoder.Close()`, the call that flushes the output.
  - `export --output`, the schema manager's file copy, shell-rc appends in
    `setup`, and the stdin temp copy now report a failed `Close` (failed flush).
  - A failed `ROLLBACK` in `WithCatalogTx`/`WithFactTx` returned a connection
    with an open transaction to the pool; `rollbackConn` now marks it bad.
  - The doctor's reserved-namespace and orphaned-file scans swallowed every walk
    error; they now report a warning (a missing root still counts as empty).
  - `repo init` ignored a failed `.gitkeep` write.
  - The embedded bootstrap-schema walk now panics with context on a build defect
    instead of returning a silently partial package set.
  - `schema reinfer` prompts read through `readYes(rootCmd.InOrStdin())`;
    `idgen` parses hash prefixes with `strconv.ParseUint`.
- **staticcheck — 76**: 61 `WriteString(fmt.Sprintf(...))` → `fmt.Fprintf`,
  De Morgan / tagged switch / struct conversion quickfixes, a redundant nil check,
  an empty branch, and the deprecated `cue.Iterator.Label` (3 sites) →
  `Selector().Unquoted()` for regular fields, `Selector().String()` for
  definitions and hidden fields.
- **unused — 11**: removed dead `recordIdentity`, `localPluginDir`,
  `canonicalArgs`, `EnhancedImporter.copyFile`, `extractPackageFromPath`,
  `convergeOutcome`, `listCollectionType`, `Inferrer.ctx`, a test-suite
  `forceCleanup` and smoke `mustNotContain`; `schemagen`'s local `schemaRef` no
  longer carries an unread `pkgPath`.
- **gofmt — 6**: test files under `test/integration` and `test/system`.
- **sqlclosecheck — 3**: read loops that closed rows by hand (so the caller could
  issue more statements on the same transaction) moved into helpers with
  `defer rows.Close()` (`unsequencedFactEvents`, `snapshotMemberIDs`, and the
  fuzz test's `prepareOnly`).

## CI and Makefile

- `make lint` and the new `make vulncheck` run golangci-lint v2.14.0 and
  govulncheck v1.8.0 via `go run` at pinned versions, so a locally installed
  binary built with an older Go cannot drift from CI. `make ci` already depends
  on `lint`.
- `.github/workflows/go.yml` gains `lint` and `vulncheck` jobs.

## Verification

`CGO_ENABLED=0 go build ./...`, `go vet ./...`, `go vet -tags=smoke ./test/...`,
`CGO_ENABLED=0 go test -count=1 ./...`, `go test -race -count=1 ./...`,
`make lint` (0 issues) and `make vulncheck` (no vulnerabilities) all pass.

No public API changes.
