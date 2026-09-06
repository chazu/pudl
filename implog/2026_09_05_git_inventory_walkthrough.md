# Self-contained Git inventory and drift walkthrough

Ticket: `pudl-sjk` (prerequisite `pudl-wby`; unblocks `pudl-qrl`).

## Delivered behavior

The getting-started guide now supplies every artifact needed for a useful first
model run. `examples/git-inventory/` contains a CUE model, a Python observer using
mu's NDJSON protocol, and two
inventories for the fictional `local/demo` repository. Setup copies them into a
fresh temporary Git repository, initializes PUDL, and creates a local mu root
with an explicit local disk cache (mu otherwise defaults to `~/.mu/cache`).
The observer uses mu's local `command` form; the `script` form would install a
global plugin copy. PUDL resolves the records' declared `git.repository` resource
type to `pudl/git.#GitRepository`, which the acceptance test verifies.
No external account or credentials are required; prerequisites are checked before
initialization, and mu v0.3.5 is the tested compatibility version.

The baseline expects and observes default branch `main`. Replacing the supplied
inventory with the changed fixture observes `release` and produces exactly one
finding: `git.repository/local/demo (changed): default_branch: release → want main`.
Repeating keeps that finding and creates a new snapshot with the same record.
The guide follows persisted run and snapshot IDs through `run report`, `list`,
and `show --raw`, including replay of the original clean report.

The guide explains existing report behavior: a successful observe-only run can
have `ok: true` and `drift.clean: false`; populate counts newly stored records,
so deduplicated repeats show zero while their snapshot still has one member.
Listing snapshot records uses `--origin pudl-run` to override the workspace-name
import filter. Richer report behavior remains in `pudl-qrl`.

## Decoder repair

The real walkthrough initially failed `model validate`: `decodeDesired` used
`Selector.String()`, which retains CUE quotes around labels such as `"_schema"`.
The resulting Go key included literal quotes, losing the routing tag. It also
enumerated absent optional constraints and introduced a `depends_on?` field.
Decoding now uses the unquoted name of string labels and excludes absent optional
fields, while retaining the existing hidden-field handling. A focused regression
checks exact desired-record keys; the CLI walkthrough covers validation and drift.

## Public interface

- New runnable example: `git-inventory`, installed by the documented commands.
- New check: `make test-git-walkthrough`, also run in CI on pushes and PRs.
- Existing `pudl model validate`, `run`, `run report`, `list`, and `show` command
  interfaces and report JSON shapes remain unchanged.
- Desired CUE records now decode quoted field names correctly and omit unset
  optional fields. No new Go API is introduced.

## Validation

The smoke test executes the marked guide blocks verbatim with real mu in a fresh
workspace, including paths containing spaces. It asserts baseline/change/repeat
verdicts, distinct run/snapshot IDs, exact persisted report replay, original raw
record values, built-in schema assignment, and local artifact paths. Global PUDL
and mu state must remain absent. Test workspaces are cleaned up.
The walkthrough also passed with a separately built, published mu v0.3.5 binary,
in addition to the installed development build.

Passed with the repository's pinned Go toolchain via `mise exec --`:

- `make test-git-walkthrough`
- `make test-kick-tires`
- `go test ./...`
- `go test -race -gcflags=all=-d=checkptr=0 ./...`
- `go vet ./...`
- `go build ./...`
- `go run ./internal/skills/gen -check`

The smoke builds a fresh CLI. The installed user CLI is not replaced by this task.
