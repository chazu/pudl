# Pulumi pack: one head key set for graph_detail (2026-11-16)

Fixes `pudl-a7aa`: with the Pulumi pack installed next to a pack whose
`graph_detail` rules carry `rank` (the Argo CD pack), `pudl doctor`
reported a Datalog rule error. The core requires every rule of a relation
to use the same head keys (`datalog.RuleSetProblems`, head key check in
`internal/datalog/rule_check.go`), also across packs. The core does not
require `rank` itself: `graph_detail(resource_id, section, key, value,
rank?)`. But the Pulumi pack's own component row has `rank`, and the Argo
CD pack's rows all have it, so every Pulumi row must have it too.

## Change

- `packs/pulumi/schema/pudl/rules/pulumi_pack.cue`: the one-value rows
  (`stack`, `type`, `repo`, `root`, `commit`, `commit note`) get `rank: 0`.
  The `component` row keeps `rank: "$D"` (the depth). A comment states the
  rule. The GCP and AWS packs write no `graph_detail` rules.
- `cmd/pulumi_pack_doctor_test.go`: `TestPulumiPackRulesPassDoctorWithOtherPacks`
  installs `pulumi`, `gcp`, `k8s` and `argocd` in one command and requires
  the doctor check "Datalog Rules" to be `ok`. Without the fix it fails
  with the head key error.

No public API changes. The inspector rows are the same and in the same
order: a missing rank was 0 already (`TestPulumiPackLinksToGCP`).
