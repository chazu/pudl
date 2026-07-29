# Swamp parity slices before cross-model value wiring

**Date:** 2026-07-27
**Design:** `docs/design/2026-07-27-swamp-parity-roadmap.md`

## Outcome

Implemented and verified the bounded PUDL/mu utility and UX slices selected by
the roadmap's review and grilling pass. Cross-model value wiring remains
explicitly deferred.

## Public surface

- `pudl model new <name> --populate plugin:<name> [--input key=value]`
- `pudl model populator new <model>`
- `pudl rule new <name>`
- `pudl run --populate plugin:<name> [--input key=value]` for an in-memory,
  observe-only model
- `pudl help --json` and `pudl model describe <name> --json`
- `pudl run report [<run-id>] [--json]`
- `pudl run --converge --require-approval`, `pudl run resume|approve`, and
  `pudl run reject`
- `pudl guide troubleshooting|memory` and the generated `pudl-core` skill
  routing
- advisory `pudl hooks suggest` for raw Kubernetes/AWS/Terraform reads

## Data and integration changes

- Observe schema routing now handles provider namespaces with multiple dots,
  acronym names, loaded-schema existence, and CUE inference fallback. Missing
  schema context now produces the generic observe schema instead of an
  unverified reference.
- Added the AWS EC2 `pudl/aws.#Instance` bootstrap schema and catalog entry.
- Added the open `pudl/k8s.#Resource` inventory envelope and bootstrap copy
  registration.
- Added durable `run_reports` and `run_approvals` catalog tables with idempotent
  migrations. Reports retain run/snapshot identifiers; approval requests retain
  the model and converge options.
- Added mu k8s `inventory` observe mode with kind and namespace selection while
  preserving the existing differential observe path.

## Verification

- `mise exec -- go test ./...`
- `mise exec -- go vet ./...`
- `mise exec -- go build ./...`
- From mu: `mise exec -- go test ./...`, `mise exec -- go build ./...`, and
  `bb plugins/k8s/plugin.bb </dev/null`
- Deterministic fake-`kubectl` inventory smoke produced typed
  `_schema: "k8s.resource"` records.
- Isolated fake-mu ad-hoc smoke produced one record, persisted/retrieved a run
  report, and left no ad-hoc model definition.
- Focused command/catalog tests cover the help tree, model description, report
  persistence, report failure rendering, approval terminal state, and PUDL
  ingest tests cover typed AWS/Kubernetes routing and inference fallback.

## Intentional limits

The approval slice is request-level: it prevents mutation until explicit
resume, then re-enters the normal observe-before-apply path. It does not yet
persist exact plan identity or requester attribution. Live cloud/cluster
credentials and optional CUE registry dependencies were not exercised in this
environment. The next design must begin with one concrete cross-model value
flow and must not introduce interpolation or lazy catalog functions by
assumption.
