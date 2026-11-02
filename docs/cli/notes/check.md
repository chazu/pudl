**A check observes and reports; it never changes anything.** For each named
model it populates the catalog, compares `desired` with what was observed,
evaluates checks and records the result. Apply changes with the tool that owns
them, then check again to verify.

**Exactly the named models.** `pudl check a b c` runs `a`, `b` and `c` and
nothing else. A model runs after every named model it lists in `depends_on`;
dependencies that are not named add no ordering and are never run. When a
named producer does not succeed, its dependents are recorded `blocked` and do
not run; independent models continue. The order and the edges are recorded in
the report (`ordered_members`, `edges`, `plan_digest`).

**One report.** A check records one report whether it names one model or
several: an aggregate `summary` (execution, conformity, checks, verification),
and one entry per model under `members` with its result, run ID, summary and
full `report` (populate, drift, checks). Read it again with
[`pudl report`](#pudl-report). Under `--json` the check writes that report as
its only output. Human output shows each model's findings as it concludes, then
the summary; internal catalog keys such as `//models/<name>` appear only in the
JSON evidence.

**Catalog replay is not an observation.** `--from-catalog` set-diffs `desired`
against records already in the catalog, so its verdict describes what was
*recorded*, not what is live. A replay therefore never writes a model status;
the model keeps the verdict of its last real observation. The scope is
mandatory because there is no way to infer which ingested records belong to a
model, and a replay names exactly one model. A model with `desired` state
populates its own snapshot and compares against it without `--from-catalog`.

**Interrupting a check.** The first Ctrl-C (SIGINT or SIGTERM) stops the check
cleanly: the running command receives SIGTERM and has 10 seconds to finish
(then it is killed), temporary workspaces are removed, the model in progress
concludes `cancelled`, the remaining models are recorded `cancelled`, and the
check's status is `cancelled`. A second Ctrl-C exits immediately (status 130),
still removing temporary workspaces. Check again to continue. A populate
`timeout` expiry is recorded as `failed`, not `cancelled`.

**Exit status.** By default `pudl check` exits 0 whenever every model
completed, including when it found drift; the report says what it found. A
failing fail-severity check or any other error exits nonzero.

With `--detailed-exitcode` the exit status is the result, so a script or CI job
can gate on it without parsing the report:

| Exit | Meaning |
|------|---------|
| `0` | Clean: no drift, every fail-severity check passed |
| `2` | Findings: drift (including a `--from-catalog` replay's) or a failing fail-severity check — and nothing else went wrong |
| `1` | Error: the check could not establish the answer (bad flags, unknown model, populate failure) |

Under this flag every error exits 1, so `2` always means findings. When several
models fail, the check exits 2 only when every failed model failed on its
fail-severity checks alone; models blocked or cancelled behind them do not
count as separate errors.

**Command populate.** A `#CommandObserve` arm runs plain commands that print
JSON (an array of records, or a stream of objects) — `gcloud`, `kubectl`,
`aws … --format=json` — directly, with no shell. Fan out with a CUE
comprehension and stamp fields the tool omits with `set`:

```cue
populate: {
	schema: "pudl/gcp.#Firewall"     // optional; routes records like import --schema
	runs: [for p in ["prod-a", "prod-b"] {
		argv: ["gcloud", "compute", "firewall-rules", "list", "--project=\(p)", "--format=json"]
		set: project: p
	}]
}
```

The environment is inherited (cloud CLI credentials work). Any failing run
fails the populate phase and nothing is ingested; the error quotes the end of
its stderr. argv and `set` are recorded with the model: keep secrets out.
`pudl check --populate 'command:<cmdline>'` checks one ad hoc.

**Checks-only models.** A model without `populate` evaluates its checks over
the catalog as it is — e.g. data brought in with `pudl import` — after syncing
projected facts ([projection](projection.md)). It cannot declare `desired`.

**Retired commands, fields and flags.** `pudl check` replaces `pudl run` and
`pudl run set`. `--converge` and its flags, `--max-observation-age`, and the
model fields `converge`, `plugins`, `sealed_inputs`, `sealed_outputs`, `inputs`
and `bindings`, and the `#PluginObserve` and `#EweTarget` populate arms were
removed. A model that uses one fails to load with an error naming the field; see
[retired commands](retired-commands.md).
