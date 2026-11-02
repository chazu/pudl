# Declared-empty collector errors: `populate.empty_when` (pudl-c2r)

A collector run whose error means "no resources here" is no longer a failed
input. Before, `gcloud compute ... list` in a project with the Compute Engine
API disabled (exit 1, `reason: SERVICE_DISABLED` on stderr) was a failed
input, so in an organization where most projects do not use Compute every
compute snapshot was incomplete, its checks unknown, and a checks-only model
depending on the collector was `blocked by unsuccessful prerequisite`.

## Semantics

- **`populate.empty_when`** lists Go RE2 regular expressions. They are
  matched against the stderr tail (last 4 KiB) of a run that **exited
  non-zero on its own**. The first matching pattern wins.
- **A match is an empty input**: observed with zero records. It is not in
  `failed_inputs`, so the existing rules all treat it as a successful
  observation. The snapshot stays complete (no errored observe result is
  added), so its scope can close. The model's outcome is not
  `errFailedInputs`, so the model succeeds and dependents are not blocked.
  `markFailedInputChecks` does not mark checks unknown. `fail_fast` does
  not trip.
- **Never empty**: a timeout (even if the command traps SIGTERM and exits
  1), a signal, a command that could not start, undecodable stdout (exit 0),
  or any failure no pattern matches (for example `PERMISSION_DENIED`). These
  stay failed inputs, as before. Nothing unmatched is swallowed.
- Only `runs` are matched. A failing `each` enumeration still fails the
  populate.
- Mixed outcomes compose: empty inputs plus a failed input give an
  incomplete snapshot and `1 of N runs failed`. Every run empty ingests an
  empty, complete snapshot. "All runs failed, nothing ingested" counts only
  failed runs.
- **Validation at load**: `DecodeValue` compiles the patterns, so `pudl model
  validate` and every load reject a bad one, naming the field:
  `model "fw": populate.empty_when[1] "(unclosed" is not a valid regular
  expression: …`. CUE rejects an empty string (`string & !=""`).
  `populate.Run` compiles them again for models built in code.

### Why match only a non-zero exit

A timeout is never a definite answer, whatever stderr said before it was
cut off. To tell it apart from a command that exits 1 after SIGTERM,
`execute` now wraps the stop reason with `%w` instead of `%v`, and
`attempt` checks `errors.Is(err, context.DeadlineExceeded)`. Undecodable
stdout means the command claimed success, so stderr is not consulted.

## Report

JSON, `populate.empty_inputs` (omitted when empty):

```json
"empty_inputs": [
  {"run": 1, "item": "proj-b", "argv": ["gcloud", "compute", "firewall-rules", "list", "--project=proj-b", "--format=json"],
   "exit_status": 1, "pattern": "SERVICE_DISABLED", "reason": "reason: SERVICE_DISABLED"}
]
```

`reason` is the trimmed stderr line containing the match, and `pattern` is
the `empty_when` entry that matched. `item` is omitted without `each`. The
shape follows `failed_inputs` (`run`, `item`, `argv`, `exit_status`), so
consumers can label both the same way.

Markdown, before the failed inputs:

```
- empty inputs: 1 of 3 runs (matched empty_when; observed with no records)
  - proj-b: exit 1: matched "SERVICE_DISABLED"
    argv: gcloud compute firewall-rules list --project=proj-b --format=json
    stderr: reason: SERVICE_DISABLED
```

## CUE surface (`internal/systemmodel/schema.cue`)

```cue
#CommandObserve: {
	...
	empty_when?: [...string & !=""]
}
```

## GCP pack

`_#GCPLive` (every live collector) sets `empty_when: ["SERVICE_DISABLED"]`,
and the `#CommandObserve` comment above `_#GCPCollector` says why. The
fixture collectors are unchanged (they run `cat`). The
`gcp-network-hygiene-live` example model sets it too. The KMS key helper
script (`set -eu`) exits with gcloud's status and passes its stderr through,
so a disabled KMS API is matched there as well.

## Public API

- `internal/systemmodel`: `Populate.EmptyWhen []string` (`json:"empty_when"`);
  `Populate.EmptyPatterns() ([]*regexp.Regexp, error)`; `DecodeValue`
  validates it; `Populate.IsZero` accounts for it.
- `internal/populate`: `Result.Empty []EmptyInput`; `EmptyInput{Run, Item,
  Argv, ExitStatus, Pattern, Reason}` with `Label()`. `Run` returns an error
  for an invalid pattern. `FailedInput` gained an unexported `exited` flag
  (not serialized).
- `internal/checkreport`: `Populate.EmptyInputs []populate.EmptyInput`
  (`json:"empty_inputs,omitempty"`), rendered by `writeEmptyInputs`.
- `internal/operation`: `runPopulate` copies `res.Empty` into the summary.
  No change to completeness, success or blocking code: those key on
  `FailedInputs`, which no longer contains declared-empty runs.

## Tests

- `internal/populate/empty_test.go`: a matched non-zero exit is empty with
  its pattern and line; an unmatched failure and undecodable stdout stay
  failed; a timeout whose command exits 1 on SIGTERM with matching stderr
  stays failed; `fail_fast` ignores empty inputs; a bad pattern fails `Run`;
  `lineAt`.
- `internal/systemmodel/empty_when_test.go`: decode, a bad regex is a load
  error naming `populate.empty_when[1]`, an empty pattern is rejected.
- `internal/operation/empty_inputs_test.go` (three enumerated projects, p2
  failing with gcloud-like stderr): with `empty_when` the snapshot is
  complete with 2 records, 1 empty input, no failed inputs, checks pass, the
  model succeeds, and the markdown lists it. Without `empty_when` it is a
  failed input and the snapshot incomplete. A `PERMISSION_DENIED` failure
  with `empty_when` set stays failed. A checks-only model that depends on
  the collector succeeds rather than being blocked.
- `cmd/gcp_pack_live_test.go`: the fake gcloud takes `$GCLOUD_PROJECTS`,
  and compute commands in `proj-nocompute` fail with `SERVICE_DISABLED`
  (exit 1). `gcp-firewalls` reports that project as an empty input with a
  complete snapshot, and `gcp-firewalls gcp-addresses gcp-routers
  gcp-network-hygiene-checks` all succeed.

## Docs

`#CommandObserve` comment in `schema.cue`, `cmd/guide/observe.txt`,
`docs/observation-scope.md` (field table and an "Empty inputs" section),
`docs/governance.md`, `examples/gcp/README.md`, and the `pudl-core` skill
(regenerated embedded copy). `docs/cli-reference.md` is unchanged.

## Line deltas

`git diff --numstat`, Go files:

| | Added | Removed | Net |
| --- | --- | --- | --- |
| Production | +150 | −11 | +139 |
| Tests | +350 | −1 | +349 |

Plus `schema.cue` +11/−1 and the pack models +11/−1.

## Verification

`CGO_ENABLED=0 go test ./...` passes (`TestCommandCancelSendsSIGTERM`, the
known flake, failed once in the full run and passed alone).
`CGO_ENABLED=0 mise exec -- make lint` reports 0 issues; `make check-docs
check-skills` pass.
