**Catalog replay is not an observation.** `--from-catalog` set-diffs `desired`
against records already in the catalog, so its verdict describes what was
*recorded*, not what is live — the records may predate the last apply. A replay
therefore never promotes resources to `clean` and never writes a model status;
the model keeps the verdict of its last real observation. The scope is mandatory
because there is no way to infer which ingested records belong to a model:
records ingested by `pudl mu ingest-observe` carry whatever target their observer
reported. Inventory observers — `#EweTarget` or `#PluginObserve` with
`differential: false` — route to inventory drift without `--from-catalog`, and
populate their own snapshot to compare against.

> **Host credentials for converge plugins.** mu runs converge actions with a
> hermetic environment — it does **not** inherit your shell's `HOME` or
> `KUBECONFIG`. A plugin that needs host credentials must get them through the
> model's `converge.input`, since pudl carries no domain knowledge to inject
> them. For the **k8s** plugin set `input.kubeconfig` to an absolute path:
>
> ```cue
> converge: #PluginPlan & {
>     plugin: "k8s"
>     input: {namespace: "...", context: "...", kubeconfig: "/abs/path/kubeconfig"}
> }
> ```
>
> Without it, apply fails with `context "…" does not exist` (kubectl falls back
> to an empty config because it can't find `~/.kube/config`). Observe is
> unaffected — it runs inside the plugin process, which keeps the full env.

**Interrupting a run.** The first Ctrl-C (SIGINT or SIGTERM) stops the run
cleanly: mu receives SIGTERM and has 10 seconds to finish (then it is killed),
temporary workspaces are removed, and the run is recorded with completion status
`cancelled`. If an apply may have been in flight, the run is also marked
needs-verification, exactly as a lost receipt is. A second Ctrl-C exits
immediately (status 130), still removing temporary workspaces. There is no
resume: re-run the model. A `--mu-timeout` expiry is recorded as `failed`, not
`cancelled`.

**Exit status.** By default `pudl run` exits 0 whenever the run completed,
including when it found drift; the report says what it found. A failing
fail-severity check, a convergence failure, or any other error exits nonzero.

With `--detailed-exitcode` the exit status is the result, so a script or CI job
can gate on it without parsing the report:

| Exit | Meaning |
|------|---------|
| `0` | Clean: no drift, no pending changes, every fail-severity check passed |
| `2` | Findings: drift (including a `--from-catalog` replay's), changes a `--dry-run` would apply, or a failing fail-severity check — and nothing else went wrong |
| `1` | Error: the run could not establish the answer (bad flags, unknown model, observe or convergence failure) |

A successful `--converge` that ended clean exits 0. A run paused by
`--require-approval` exits 0. Under this flag every error exits 1, so `2`
always means findings.

**Producers.** A standalone `pudl run` never starts another model
automatically. If the model has a plain binding, PUDL reuses the latest eligible
successful producer snapshot in the same workspace; use `pudl run set` to
observe and pin producers in the current operation.

**Apply budgets.** `--max-iters` bounds the applies inside one process;
`--max-applies` bounds them across processes. Without the second, a model that
cannot converge applies `--max-iters` times on every scheduled run — and a
crash-loop supervisor grants a fresh cap on every restart — so the apply rate is
unbounded. The durable budget counts every successful apply the moment it
happens, and resets to full the first time an **unscoped** run observes the
model clean. A model that drifts and is fixed on each run therefore ends every
run clean and never approaches it; only a model that applies and *still* is not
clean accumulates. An exhausted budget still observes (that is how it resets)
and refuses only the apply, reporting `failed (apply_budget_exhausted)`. A
scoped (`--only`) clean does not reset it, for the same reason a scoped ∅ does
not write `clean` to the model row.

**Selecting resources with `--only`.** `--only` accepts one or more
comma-separated exact selectors. A selector may be an **identity** key — the
desired resource `name`, `id`, `path`, or `target`, or `metadata.name` — which
names exactly one resource, or a **type** key — `_schema`, `schema`,
`definition`, or `kind` — which selects every resource of that type. The short
name after a schema's `#` is also accepted.

A selector must resolve unambiguously. It is an error for one selector to match
some resources by identity and others by type (for example `--only nginx` where
one resource is *named* `nginx` and another has `kind: nginx`), or to match
several resources by identity. Both cases would otherwise pull resources the
operator never named into converge scope.

Declared resource dependencies are included transitively. A dependency must
resolve to exactly one resource, so a dependency naming a type is an error.
The selector set is validated before convergence side effects begin.

The scoped model is what every phase consumes — planning, execution, report
scope, resource promotion and checks all see the selected resources, not the
model's full desired set.

A scoped run does not write `clean` to the model's own status row. A ∅ over the
named resources is a statement about those resources; the ones left out of scope
were never observed, so generalizing it would let `pudl status`, `pudl model
list` and `--check-upstream` read whole-model "in sync" off a partial run. The
model row is left `unknown` instead, and the run row records the real verdict
plus a note naming the scope. `drifted` and `failed` *are* written: a defect
found in a subset is a defect in the model. Re-run without `--only` to establish
a whole-model `clean`.

**Command populate.** A `#CommandObserve` arm runs plain commands that print
JSON (an array of records, or a stream of objects) — `gcloud`, `kubectl`,
`aws … --format=json` — directly, with no shell, no mu and no plugin protocol.
Fan out with a CUE comprehension and stamp fields the tool omits with `set`:

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
projected facts ([projection](projection.md)). It cannot declare
`desired` or `converge`.
