**A run observes and reports; it never changes anything.** It populates the
catalog, compares `desired` with what was observed, evaluates checks and
records a report. Apply changes with the tool that owns them, then run again to
verify.

**Catalog replay is not an observation.** `--from-catalog` set-diffs `desired`
against records already in the catalog, so its verdict describes what was
*recorded*, not what is live. A replay therefore never writes a model status;
the model keeps the verdict of its last real observation. The scope is
mandatory because there is no way to infer which ingested records belong to a
model. A model with `desired` state populates its own snapshot and compares
against it without `--from-catalog`.

**Interrupting a run.** The first Ctrl-C (SIGINT or SIGTERM) stops the run
cleanly: the running command receives SIGTERM and has 10 seconds to finish
(then it is killed), temporary workspaces are removed, and the run is recorded
with completion status `cancelled`. A second Ctrl-C exits immediately (status
130), still removing temporary workspaces. Re-run the model to continue. A
populate `timeout` expiry is recorded as `failed`, not `cancelled`.

**Exit status.** By default `pudl run` exits 0 whenever the run completed,
including when it found drift; the report says what it found. A failing
fail-severity check or any other error exits nonzero.

With `--detailed-exitcode` the exit status is the result, so a script or CI job
can gate on it without parsing the report:

| Exit | Meaning |
|------|---------|
| `0` | Clean: no drift, every fail-severity check passed |
| `2` | Findings: drift (including a `--from-catalog` replay's) or a failing fail-severity check — and nothing else went wrong |
| `1` | Error: the run could not establish the answer (bad flags, unknown model, populate failure) |

Under this flag every error exits 1, so `2` always means findings.

**Producers.** A standalone `pudl run` never starts another model
automatically. If the model has a plain binding, PUDL reuses the latest eligible
successful producer snapshot in the same workspace; use `pudl run set` to
observe and pin producers in the current operation.

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
`pudl run --populate 'command:<cmdline>'` runs one ad hoc.

**Checks-only models.** A model without `populate` evaluates its checks over
the catalog as it is — e.g. data brought in with `pudl import` — after syncing
projected facts ([projection](projection.md)). It cannot declare `desired`.

**Retired fields and flags.** `--converge` and its flags, and the model fields
`converge`, `plugins`, `sealed_inputs` and `sealed_outputs`, and the
`#PluginObserve` and `#EweTarget` populate arms were removed. A model that uses
one fails to load with an error naming the field; see
[retired commands](retired-commands.md#convergence-and-mu-integration).
