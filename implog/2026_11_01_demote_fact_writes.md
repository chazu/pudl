# Fact-store write commands demoted from everyday help (pudl-4fx.3)

SREs read the fact store through `pudl query`. `pudl facts add`, `retract` and
`invalidate` (writes and lifecycle changes on two time axes) no longer appear in
everyday help. They still run unchanged, and the CLI reference documents them.
`pudl prime` stays the agent entrypoint and is not merged into `pudl guide`.

## Mechanism

Cobra `Hidden` is the lightest option. An "advanced" help group was rejected:
with groups, Cobra lists every ungrouped subcommand under "Additional Commands",
so each `facts` subcommand would need a group, including `reproject`, which
another change is folding into `doctor --fix`.

Plain `Hidden` would also have removed the commands from `pudl help --json` and
the CLI reference, because both skip hidden commands. Both also need to skip
Cobra's own hidden `__complete` commands. To tell the two kinds apart, the
demoted commands carry an explicit marker:

- `internal/cliadvanced` (new, cobra-only):
  - `Mark(c *cobra.Command)` sets `Hidden` and the `pudl.advanced` annotation.
  - `Is(c *cobra.Command) bool` reports the marker.

  The marker is a separate package because `cmd` cannot import
  `internal/clidocs`: the clidocs in-package test imports `cmd`, so the import
  would create a cycle.
- `pudl help --json` keeps advanced commands in the tree with
  `"advanced": true`. Other hidden commands are still omitted. The help golden
  file therefore still pins the full surface.
- `internal/clidocs` documents advanced commands after the everyday sections,
  under a `## Advanced commands` heading with its own index. A parent's section
  lists them under "Advanced subcommands". The notes check covers them too.
- `TestJSONOutputContractClassifiesEveryCommand` still walks advanced commands.

## Command decisions

| Command | Everyday help | Reason |
|---|---|---|
| `facts add` | hidden (advanced) | Asserts arbitrary facts; ordinary facts come from import projection and runs |
| `facts retract` | hidden (advanced) | Lifecycle operation on transaction time |
| `facts invalidate` | hidden (advanced) | Lifecycle operation on valid time |
| `facts list` | visible | Read-only inspection by relation |
| `facts show` | visible | Read-only inspection of one fact and its versions |
| `facts search` | visible | Read-only full-text lookup when the relation is unknown |
| `facts stats` | visible | Read-only overview of what the store holds |
| `facts reproject` | untouched | Owned by the doctor `--fix` change |

The four visible commands only read. They answer "what is in the store" when a
`pudl query` relation is unknown. `--as-of-*` stays on `facts list` as an
opt-in flag.

## Text changes

- `pudl facts` long help lists list/show/search/stats/reproject. It says that
  add/retract/invalidate are advanced and points to `pudl guide facts`.
- `pudl prime`: the "Facts and rules" section leads with `pudl query` and the
  inspection commands. It describes the three writes as advanced commands,
  hidden from everyday help.
- `pudl guide`: removed `facts add` from the overview's day-to-day verbs and
  from the agents topic's command block. The facts topic notes that the three
  writes are advanced commands.
- `docs/retired-commands.md`: notes that the commands are demoted, not retired.
- Regenerated `cmd/testdata/help.golden.json` and `docs/cli-reference.md`.
  Skills needed no change (`make check-skills` passes).

`pkg/factstore` is unchanged (`AddFact`, `RetractFact`, `InvalidateFact`).

## Line delta

- Production Go: +148 / -36 (net +112). About 30 of these lines are help text.
- Test Go: +27 / -1 (net +26).

## Validation

- `CGO_ENABLED=0 go test ./...`: all packages pass.
- `make check-docs` and `make check-skills` pass.
- `CGO_ENABLED=0 mise exec -- make lint`: 0 issues. Plain `make lint` fails
  locally while linking the golangci-lint binary: the host clang rejects the
  macOS 27.2 SDK `.tbd` files. This is unrelated to pudl code.
