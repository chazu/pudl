# Per-input observation scope; complete k8s collectors (2026-11-16)

`pudl-u29`. Design: `docs/design/2026-11-16-per-input-scope.md`. The k8s
design doc's "Observation scope" section now points to it.

## Behaviour

- `observation.scope` may contain `{{item}}`. With `each`, every
  enumerated input is its own observation snapshot: scope with the item
  substituted, only that input's records, snapshot ID
  `<run snapshot ID>_<n>` (1-based input position).
- Completeness and gone marking (`CloseScopeLeavers`, snapshot compare)
  are per input scope. A failed input makes only its own snapshot
  incomplete (its failed run recorded as an errored target). Scopes the
  run did not observe are untouched.
- A scope without `{{item}}` is unchanged: one snapshot, `input` empty,
  no `populate.observations`.
- Load-time errors: a per-input scope without `each` (command or HTTP),
  with `fallback`, or on a model with `desired`.
- Evidence: `current` and `model:<name>` select every input snapshot of
  the latest run; `scope:<scope>` names one input's scope. A check is one
  verdict over those scopes; any incomplete one makes it `unknown`.
- Retention: pruning protects the newest successful snapshot of each
  input scope; a newer per-input snapshot stops protecting the model's
  older fixed-scope one. The previous-successful lookup of a per-input
  snapshot stays within its scope (tombstones now carry the scope).
- Migration 28 (`observation_input`): `observe_snapshots.input`,
  `observe_snapshot_tombstones.scope`. Old fixed-scope k8s snapshots are
  left as they are (never marked gone; no longer selected by `model:`).

## Kubernetes pack

The 8 live collectors (`k8s-cluster`, `-deployments`, `-statefulsets`,
`-daemonsets`, `-services`, `-ingresses`, `-namespaces`, `-nodes`)
declare `scope: "k8s/{{item}}/<kind>", complete: true`. `k8s-cluster`
now enumerates the host with `_k8sClusterEach` too (its run sets
`host: "{{item}}"`, the value the helper already prints). The live
posture family reaches pass/fail verdicts.

## API

- `systemmodel.Observation.PerInput()`, `Observation.ScopeFor(item)`
  (`internal/systemmodel/observation.go`).
- `populate.Result.InputRecords [][]any` (parallel to `Inputs`).
- `observe.ObserveIngest.Input`, `database.ObserveSnapshot.Input`.
- `database.CatalogDB.ModelObservations(model) ([]ObserveSnapshot, error)`.
- `checkreport.Populate.Observations []InputObservation`
  (`Input`, `Scope`, `SnapshotID`, `Records`, `Complete`),
  `Populate.SnapshotIDs()`.
- `evidence.Request.Current []string`, `checks.Request.CurrentSnapshots []string`
  (were single strings).
- `pudl snapshot show` prints `input:` for a per-input snapshot.

## Tests

- `internal/systemmodel/observation_test.go`, `internal/populate`
  `TestRunKeepsEachInputsRecords`,
  `internal/database/observe_snapshots_input_test.go`.
- `internal/operation/populate_inputs_test.go`: one failed input of two;
  context switch A→B closes nothing of A; re-observing A without a thing
  closes its policy result and compares `removed`; fixed scope unchanged.
- `cmd/k8s_pack_scope_test.go`: live posture family verdicts; context
  switch with the fake kubectl (`K8S_SERVER` selects the API server).
