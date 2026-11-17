# Per-input observation scope

Status: accepted, 2026-11-16 (`pudl-u29`).

## Problem

A collector's `observation.scope` is a fixed string. A collector that
reads a different target on each run, such as the Kubernetes pack's
collectors that read the current kube context, cannot be complete. A
complete observation scoped `k8s/deployments` of cluster B would mark
cluster A's deployments gone. So those collectors declare
`complete: false`, and every check over them is `unknown`
(`incomplete_observation`).

## Syntax

A scope can contain the `{{item}}` placeholder, the same placeholder
that `each` substitutes in `argv` and `set`:

```cue
observation: {scope: "k8s/{{item}}/deployments", complete: true}
populate: {
	each: {argv: ["sh", "cluster.sh"], select: "host"}
	runs: [{argv: ["kubectl", "get", "deployments.apps", "-A", "-o", "json"], set: cluster: "{{item}}"}]
}
```

A scope without `{{item}}` behaves exactly as before. No existing pack
uses the placeholder, so the GCP, AWS, Argo CD and Pulumi packs do not
change.

A model load fails when a scope uses `{{item}}` and:

- the populate declares no `each` (command or HTTP): there is no item;
- the populate declares a `fallback`: the fallback's inputs are not the
  populate's inputs, so its records have no scope to go under;
- the model declares `desired`: an inventory compares `desired` with one
  snapshot (`pudl check --from-catalog` and drift read one scope).

## Semantics

The populate runs as before: one `each` enumeration, then every run for
every input. pudl then ingests **one observation snapshot per
enumerated input**, not one for the whole populate:

- **Scope.** The scope template with `{{item}}` replaced by the input,
  e.g. `k8s/198.51.100.30/deployments`.
- **Records.** Only the records of that input's runs.
- **Snapshot ID.** `<run snapshot ID>_<n>`, where `n` is the input's
  1-based position in the enumeration. The run allocates its snapshot ID
  before it observes, so every per-input ID is known before the ingest.
- **Input.** A new column `observe_snapshots.input` records the item.
  It is empty for a snapshot of a fixed scope. `pudl snapshot show`
  prints it.
- **Completeness.** Each snapshot is complete as declared, unless one of
  its own input's runs failed, or a record of that input fell back to
  another schema. A failed input makes only its own snapshot incomplete.
  That snapshot holds the input's other records (if it has more than
  one run) and the failed run as an errored target. An empty input
  (`empty_when`) is a complete snapshot with no records, as it is for a
  fixed scope.
- **Gone.** A complete snapshot closes the policy results of resources
  that an earlier complete snapshot of the **same scope** held and it
  does not (`CloseScopeLeavers`). This is unchanged code, keyed by the
  substituted scope. A snapshot of cluster B never closes anything of
  cluster A. An incomplete snapshot closes nothing.
- **Unobserved scopes.** Records under a scope that this run did not
  observe (another cluster, or an input that the enumeration no longer
  lists) stay as they are. They are not marked gone and their latest
  snapshot stays the latest of their scope.
- **All inputs failed.** Nothing is ingested and the populate fails, as
  before.
- **Run report.** `populate.observations` lists each input's snapshot:
  `input`, `scope`, `snapshot_id`, `records`, `complete`.
  `populate.snapshot_id` is empty, because no single snapshot holds the
  populate. `populate.records` and `new_records` are the totals.

### Snapshot comparison, previous snapshot, retention

- `pudl snapshot show NEW --compare OLD` already requires one scope, so it
  compares two observations of one input.
- The previous successful snapshot of a per-input snapshot is the previous
  one of the same model, workspace **and scope**. For a fixed scope the
  lookup does not change.
- Pruning protects the newest successful snapshot of each model and
  workspace. For per-input snapshots it protects the newest of each
  scope, so every cluster keeps its latest observation. A newer
  per-input snapshot also stops protecting the model's older fixed-scope
  snapshots (see Migration).

## Checks

The evidence selectors resolve as follows:

| Selector | Fixed scope | Per-input scope |
| --- | --- | --- |
| `current` | the run's snapshot | every input snapshot of the run |
| `model:<name>` | the model's newest snapshot | every input snapshot of the model's newest run |
| `scope:<scope>` | the newest snapshot of the scope | the same; name a substituted scope, e.g. `scope:k8s/198.51.100.30/deployments` |

A check is evaluated **once over the scopes the collector's latest run
observed**. Its evidence lists each of those snapshots with its scope.
So the verdict covers those scopes only:

- A pass says nothing about a scope that the latest run did not observe.
  After a context switch to cluster B, `model:k8s-deployments` selects B
  only, and the report's evidence says so.
- Every selected snapshot must be complete and within `max_age`.
  Otherwise the check is `unknown`. When one input of the run failed, the
  check is `unknown`, and it does not pass for the other scopes either.
  One check result cannot be a pass for A and unknown for B; a separate
  verdict per scope is a possible follow-up.
- A finding is resolved only by a later complete evaluation over the
  same set of evidence scopes. This is the existing `Comparable` rule.
  A run that observed only B therefore cannot resolve a finding seen in
  A: the two evaluations are not comparable.
- A family over several collectors (the posture family reads
  deployments, statefulsets and daemonsets) selects each collector's
  latest run. If those runs read different clusters, the evidence shows
  both scopes.

Run-scoped and latest-known checks of the collecting model itself do not
change. Failed inputs make them `unknown` (`markFailedInputChecks`).

## Migration

Snapshots recorded under a fixed scope (for example `k8s/deployments`)
stay as they are:

- They were incomplete, so they never closed anything. No new snapshot
  uses their scope, so nothing compares against them and nothing is
  marked gone.
- `model:` evidence selects the model's newest run, so they stop being
  evidence after the first per-input run.
- Their records keep their resource identity (`[cluster, namespace,
  name]`), so the first per-input observation of the same object makes
  the same resource current again. An object deleted before that first
  observation is not marked gone, because no complete snapshot of a
  scope ever held it. Delete it by hand, or let `pudl snapshot prune`
  remove the old snapshots.
- Pruning no longer protects them once a newer successful per-input
  snapshot of the model exists.

## Code

| Area | Change |
| --- | --- |
| `internal/systemmodel` | `Observation.ScopeFor(item)`, `PerInput()`; load-time validation above |
| `internal/populate` | `Result.InputRecords`: the records of each input, in input order |
| `internal/operation` | `runPopulate` ingests one snapshot per input when the scope is per-input |
| `internal/database` | `input` column; `ModelRunObservations`; scope-aware previous lookup and pruning |
| `internal/evidence`, `internal/checks` | `current` and `model:` resolve to several snapshots |
| `internal/checkreport` | `Populate.Observations`; completeness and evidence read them |
