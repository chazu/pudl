# Multi-project fan-out: failed inputs and enumeration (pudl-xf5.3)

A `#CommandObserve` fan-out no longer discards every project when one fails,
and its inputs can come from a command at run time instead of a CUE list.

## Failure semantics

- **A failing run is a failed input.** A run fails when it exits non-zero,
  times out, or prints something other than JSON objects. The other runs go
  on. Their records are ingested as one snapshot, as before.
- **The observation is incomplete.** Each failed input is recorded as an
  errored observe result (target `//models/<name>:populate[<run>]`). That is
  the existing completeness rule: target errors make the snapshot incomplete,
  so its scope is not closed (`CloseScopeLeavers` is skipped, policy results
  stay open), `snapshot show --compare` reports the failed input's resources
  `not-observed` rather than `removed`, and inventory drift reports missing
  desired resources as uncertain.
- **Checks are unknown, never passing.** Checks that select the snapshot
  (`current`, `model:<name>`, `scope:<name>`) are unknown through the
  existing `incomplete_observation` diagnostic. The model's own checks that
  read the catalog directly (run or latest-known scope) cannot see the failed
  input's resources either, so they are marked unknown with a
  `failed_inputs` diagnostic (`markFailedInputChecks`). Their count and
  witnesses stay in the JSON. This includes a check whose findings are all
  waived (`pudl-xf5.4`): the failed inputs could hold findings no waiver
  covers.
- **The model fails.** The run outcome is `errFailedInputs` (`populate: one or
  more inputs failed; their resources are unknown: 1 of 3 runs failed (p2)`,
  at most five inputs named). The member is `failed`, `--detailed-exitcode`
  exits 1, and dependents in the same check are blocked, as for any failed
  producer. A pure collector with no checks therefore still fails CI.
- **Every run failing ingests nothing**, as before. One failed run produces
  exactly the former error (`populate run 0 (argv): exit status 1` plus the
  stderr tail). Several produce `populate: all N runs failed, so nothing was
  ingested (the report lists them); first: …`. The report still lists them.
- **Fatal, not per-input:** a bad timeout, a failed or malformed `each`
  enumeration, exceeding the staging byte limit, and an interrupt.
- **`fail_fast: true`** restores all-or-nothing: the first failure is the
  populate's error and later runs do not start.
- **Reports.** JSON: `populate.runs`, `populate.failed_inputs[]` (`run`,
  `item`, `argv`, `exit_status` (-1 when it did not exit on its own),
  `error`, `stderr_tail`, up to 4 KiB). Markdown: `failed inputs: N of M
  runs`, then one line per input with its exit status and error, its argv when
  it was enumerated, and the last stderr line.

### Why no per-run input label

The ticket asked whether a run should declare the input it covers (for
example `input: "p1"`). It does not need to. Verdicts are per check, not per
resource, so the failed slice is never partitioned out of a check: any check
over an incomplete observation is unknown. Partitioning would need a declared
mapping from records to inputs and a per-resource unknown in the evidence
view, which neither exists nor helps a check verdict. The failed input is
already identifiable: by its enumerated `item`, and for a static run by its
argv (whose `set` usually carries the project too). Scope closure is per
snapshot, so leaving the snapshot incomplete already keeps every scope open
for the failed slice.

## Dynamic enumeration

```cue
populate: {
	each: {argv: ["gcloud", "projects", "list", "--format=json"], select: "projectId"}
	runs: [{argv: ["gcloud", "compute", "firewall-rules", "list", "--project={{item}}", "--format=json"], set: project: "{{item}}"}]
}
```

`each` runs once (same timeout, byte budget and stderr passthrough as a run).
Its stdout is decoded like run output, a JSON array or stream; each element is
a string, or with `select` (a field path, nested and quoted segments
allowed) the string or number at that path in an object. Empty items are an
error, and duplicates are dropped in order. Every run is then repeated per
item, item by item, with `{{item}}` replaced in argv and in `set`'s string
values. The report records the items as `populate.inputs`.

Why a placeholder: CUE interpolation (`"\(p)"`) is resolved when the model is
loaded and cannot see a value that exists only at run time. Re-evaluating the
model's CUE per item would need the loader's `cue.Value` kept through
orchestration. A literal placeholder keeps the CUE surface to two fields, and
CUE interpolation stays usable for everything static. Validation at load (so
`pudl model validate` catches it): with `each`, every run must use
`{{item}}`; without `each`, none may.

## CUE surface added (`internal/systemmodel/schema.cue`)

```cue
#CommandObserve: {
	...
	each?:      #Enumeration
	fail_fast?: bool | *false
}
#Enumeration: {
	argv:    [string, ...string]
	select?: string & !=""
	dir?:    string
}
```

## Public API

- `internal/populate`: `Run(ctx, systemmodel.Populate, Request) (Result,
  error)` (was `([]any, error)`); `Result{Records, Runs, Failed, Inputs}`;
  `FailedInput{Run, Item, Argv, ExitStatus, Error, Stderr}` with `Err()` and
  `Label()`.
- `internal/systemmodel`: `Populate.Each *Enumeration`, `Populate.FailFast`;
  `Enumeration{Argv, Select, Dir}`; `ItemPlaceholder = "{{item}}"`;
  `CommandRun.ForItem(item) CommandRun`. `DecodeValue` rejects a run whose use
  of the placeholder does not match `each`. `Populate.IsZero` accounts for the
  new fields.
- `internal/checkreport`: `Populate.Inputs`, `Populate.Runs`,
  `Populate.FailedInputs`; the populate section renders failed inputs; the
  plan says `N runs per enumerated input` with `each`.
- `internal/operation` (unexported): `errFailedInputs`, `failedInputsError`,
  `allInputsFailed`, `failedInputResults`, `markFailedInputChecks`.
  `runPopulate` returns its summary with the error when every run failed, so
  the report lists them.

## Tests

- `internal/populate`: failed inputs keep the rest (exit status, stderr tail,
  undecodable stdout), `fail_fast` stops at the first failure, an interrupt is
  fatal; enumeration with `select` expands, substitutes argv and `set`,
  dedupes and records inputs; a failed enumerated run names its item;
  enumeration failures (command fails, objects without `select`, missing or
  non-string field) fail the populate.
- `internal/systemmodel`: `each`/`fail_fast` decode, placeholder validation
  both ways, `ForItem` substitution.
- `internal/operation` (fixture: `cat` over `projects.json` and
  `<project>.json`): no failure gives a complete snapshot, passing checks and
  recorded inputs; deleting `p2.json` ingests p1 and p3, leaves the snapshot
  incomplete, makes latest-known, run-scoped and `current` checks unknown,
  reports `p2` (exit 1, stderr), fails the model with `errFailedInputs`, and
  `CompareSnapshots` reports p2's resource `not-observed`; every input failing
  ingests nothing and still lists them.
- The existing populate, check and smoke tests pass unchanged apart from the
  `populate.Run` signature.
- End to end with the binary, a fake `gcloud` and the README's
  `gcp-org-firewalls` model: `model validate` passes, and `check` ingests p1
  and p3, lists p2 with gcloud's permission error, records an incomplete
  snapshot and exits 1.

## Docs

`#CommandObserve` comment in `schema.cue`, `docs/cli/notes/check.md`
(regenerated `docs/cli-reference.md`), `docs/observation-scope.md`, the
`pudl-core` skill (and its embedded copy), and the GCP pack README (failure
behavior, and a "collect every project" collector with `each`). The pack's
models and fixtures are unchanged apart from one comment.

## Line deltas against `sre-focus`

`git diff --numstat sre-focus`, Go files, after the final merge of
`sre-focus`:

| | Added | Removed | Net |
| --- | --- | --- | --- |
| Production | +547 | −70 | +477 |
| Tests | +411 | −12 | +399 |

Plus `schema.cue` +33/−4.

## Verification

After the final merge of `sre-focus`: `CGO_ENABLED=0 go test ./...`,
`CGO_ENABLED=0 mise exec -- make lint` (0 issues), `mise exec -- make
check-docs check-skills test-git-walkthrough` and `go test -tags=smoke
./test/smoke/` pass.

## Follow-ups

- A family checked together with a collector that has failed inputs is
  blocked rather than evaluated to unknown. Both withhold a pass; evaluating
  it would need producer success to mean "ran" rather than "succeeded".
- The enumerated inputs are recorded in the report, not in the snapshot
  payload.
