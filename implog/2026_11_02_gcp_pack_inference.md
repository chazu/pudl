# pudl-8wr: GCP pack fixtures inferred as pudl/infra.#Service

**Date**: 2026-11-02
**Type**: Bug fix
**Status**: Complete

## Bug

Importing the GCP pack's firewall and route fixtures without `--schema`
classified them as `pudl/infra.#Service` instead of `gcp.#Firewall` /
`gcp.#Route`. Collectors pass the schema explicitly, so pack checks were
unaffected; ad-hoc imports were misclassified.

## Root cause

Two things combined:

1. **Scoring.** Candidate scoring is name-only. `infra.#Service` declares
   `identity_fields: ["name"]` and `tracked_fields: ["kind"]`; every gcloud
   record has both, so it scores 0.6. The pack types use
   `identity_fields: ["selfLink"]` with no tracked fields and score 0.5.
   `#Service` (`name: string, kind: string, ...`) accepts any such record, so
   it was tried first and won. The pack's strongest evidence, the pinned
   `kind: "compute#firewall"`, played no part in scoring. With `project`
   injected (`--set project=...`), main classified only 6 of the 15 fixture
   types correctly (networks, subnets, firewalls, routes, routers, addresses,
   DNS zones, SQL instances and Cloud Run services went to `infra.*` or
   `k8s.#Resource`).
2. **Schema contract (by design).** Raw gcloud output has no `project`;
   collectors inject it (`set: project`) and every pack type requires it
   (facts and ownership read it). A raw record therefore cannot unify with a
   pack type and correctly falls back. That part is documented, not changed.

## Fix

- `validator.SchemaMetadata.Discriminators` (new, `json:"-"`): the required
  top-level fields a schema pins to a single string, extracted by
  `literalStringFields` (`internal/validator/discriminators.go`) when the
  module loads. Defaults, disjunctions, open types and optional fields are
  excluded. List-type schemas get none.
- `inference.SelectCandidates` adds `discriminatorBonus` (0.3) to a candidate
  that already scored and whose pinned value the record carries exactly
  (`internal/inference/discriminator.go`). A pack type with matching identity
  and `kind` now scores 0.8 and is tried before the generic 0.6 schema.
  Policies and collection/item mismatches still score 0 and get no bonus.
- `examples/gcp/README.md`: documents ad-hoc import with
  `pudl import --path fw.json --set project=my-project`, and why a record
  without `project` falls back to a generic schema.

No public API change. Net production delta: +64 Go lines, 0 removed
(`discriminators.go` +34, `discriminator.go` +22, `heuristics.go` +4,
`validation_result.go` +4, `cue_loader.go` +3); README +9.

## Validation

- New `cmd/gcp_pack_inference_test.go` (`TestGCPPackFixturesInferTheirType`):
  installs the pack, imports every fixture with `--set project=example-project`
  and no `--schema`, and requires each record to land on its pack type. Fails
  on main (first failure: networks, 0 of 2 as `gcp.#Network`), passes now.
- New `internal/inference/discriminator_test.go`: a generic `#Service` vs a
  `#Firewall` with pinned `kind`; the pinned kind wins, another kind falls
  back to `#Service`; open fields are not discriminators.
- Manual: all 15 GCP fixture types classify correctly with `--set project`;
  raw SQL instances (which carry `project`) now infer `gcp.#SQLInstance`
  (were `infra.#Organization`). AWS fixture classification unchanged vs main.
- `CGO_ENABLED=0 mise exec -- go test -count=1 ./...`, `make lint` (via mise),
  `make check-docs check-skills`: all pass.
