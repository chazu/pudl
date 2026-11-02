# Policy engine and doctor fixes from the GCP pack — pudl-opz, pudl-pob

Fixes three of the engine findings recorded in
`implog/2026_11_02_gcp_content_pack.md` (1, 2 and 4) and removes the GCP
pack's workarounds for them. Finding 3 (conditional absence) is a design
limitation and keeps its documented workaround; finding 5 is `pudl-qhx`.

## pudl-opz (1): annotations lost to an undecidable comprehension

**Root cause.** `internal/policy/tree.go` `merge` walked a value's fields only
when `IncompleteKind()` included `StructKind`. A struct holding a
comprehension whose condition reads an optional field (`direction?: _`,
`if direction == "INGRESS" {...}`) is incomplete in the bare policy and its
kind is bottom, so the walk skipped the policy's whole root: `Rules` was
empty and violations were reported under their path with medium severity.
The error from `v.Fields` was ignored as well.

**Fix.**

- `merge` also walks bottom-kind values: `Fields(cue.Optional(true))` still
  enumerates their declared fields. It fails only for a value without fields
  (top or a scalar).
- `checkAnnotations` (new `annotations.go`) reads every `@pudl` attribute
  from each chain member's raw syntax, with its field path (comprehension
  bodies and embeddings declare fields of the enclosing struct; list elements
  are `[*]`), and requires the tree to carry it: the rule at that path, or
  the `absent` flag. Under a dynamic label (a pattern constraint or
  `(k):`), only the rule name is checked. An annotation the tree lacks, such
  as a field annotated only inside an undecidable comprehension, makes the
  policy fail to load:
  `sourceRanges: @pudl(rule="r", severity="high") cannot be applied: the
  field is declared only inside a comprehension whose condition depends on
  the record, or under a dynamic label; declare the field and its annotation
  outside the comprehension (...)`. Rules are never dropped silently.
- A condition the record leaves undecided (the record lacks `direction`)
  applies nothing and reports nothing, as before.

## pudl-opz (2): closed nested structs broke references

**Root cause.** Evaluation unified the record with each chain member and
dropped `field not allowed` errors. But a definition is closed at every
depth, and CUE reports extra record fields one struct level at a time; the
closedness error inside a nested struct makes references into that struct
bottom. With `privateClusterConfig: {enablePrivateNodes: true,
enablePrivateEndpoint?: _}` and a record carrying `masterIpv4CidrBlock` under
`privateClusterConfig`, `privateClusterConfig.enablePrivateEndpoint != _|_`
was false and a compliant cluster was flagged.

**Fix.** `openValidate` (new `open.go`): unify and validate; every
`field not allowed` error at a record path the member does not declare
(`declaresAt`) is removed from a copy of the record (`removeRecordPath`), and
the unification is repeated until none is left. Only the final
unification's errors become findings, since earlier ones may come from a
broken reference. No constraint of the member applies to a field it rejects
as not allowed, so pruning hides no violation; a field the member itself
adds beyond a closed embedding is not pruned. The copy is decoded with
`json.Number`, so numbers keep their exact text. Records without extra
nested fields take one unification, as before.

## pudl-pob: policies treated as family members

**Root cause.** Policies are never assigned to records, but
`CheckIdentityFieldConsistencyAt` compared a standalone policy's absent
`identity_fields` with its type's, and the projection registry gave a policy
the facts it inherits along `base_schema`, then disabled projection for it
("declares facts but no identity_fields"), so every `pudl query` warned.

**Fix.** The doctor check skips a child schema whose `IsPolicy()`; the
projection registry returns no spec for a policy, so it is neither projected
nor reported.

## GCP pack workarounds removed (`examples/gcp/schema/gcp/policies.cue`)

- `identity_fields` repeated in all six policies.
- Hidden-field defaults: `_direction`, `_disabled` (firewall), `_internal`
  (address), `_rotates` (KMS), `_privateEndpoint`, `_authorizedNetworks`
  (GKE). Replaced by plain conditionals. CUE's `&&` and `||` do not
  short-circuit over a field the record lacks (`x == _|_ || x == false` is
  undecided when `x` is absent), so a condition that treats an absent field
  as false tests the list of present values with `list.Contains`, the idiom
  the SQL policy already used: `if direction == "INGRESS" if allowed != _|_
  if !list.Contains([if disabled != _|_ {disabled}], true) {...}`,
  `if rotationPeriod == _|_ {...}`, and a `_restricted` list for GKE.
- The trailing `...` in `settings`, `backupConfiguration`, `ipConfiguration`,
  `authorizedNetworks` elements, `privateClusterConfig` and
  `workloadIdentityConfig`.
- Kept: the absence-on-a-present-field anchors (`databaseVersion`, `purpose`,
  `endpoint`) for conditional absence.

The old and new policies were compared on 23 edge records (absent
`disabled`, `addressType`, `rotationPeriod`, `enablePrivateEndpoint`,
`masterAuthorizedNetworksConfig` and its `enabled`, extra nested fields):
identical statuses, violations, unknowns and rules.

## Tests

- `internal/policy/open_test.go`: the exact reproductions.
  `TestLoad_UndecidableComprehensionKeepsRules` (rule `r` loads with severity
  high; an INGRESS world range is a `r`/high violation at `sourceRanges[*]`;
  EGRESS and a record without `direction` are compliant);
  `TestLoad_RuleOnlyInsideComprehensionFailsToLoad`;
  `TestEvaluate_NestedClosednessIsIgnored` (extra fields under
  `privateClusterConfig` stay compliant; real conflicts beside them still
  fail). All three fail on the previous code.
- `internal/doctor/checks_test.go`: a standalone policy without
  `identity_fields` passes the family check.
- `internal/projection/registry_test.go` `TestRegistrySkipsPolicies`.
- `cmd/gcp_pack_test.go`: unchanged verdicts and policy counts; adds that
  `pudl query` prints no "projection disabled" and `pudl doctor` no
  "Inconsistent identity_fields" (both fail without the pob fix).

## Public API

No exported API changes. `internal/policy` gains unexported
`checkAnnotations`, `openValidate`, `declaresAt`, `removeRecordPath`,
`(*node).at`. Policy authoring: a field annotated only inside a comprehension
whose condition reads record data now fails to load instead of losing its
rule; nested policy structs need no `...`; policies need no
`identity_fields`.

## Line counts (against `da6eb86`, the sre-focus base)

- Production Go: +353 / −30. `internal/policy`: `annotations.go` +169,
  `open.go` +89, `paths.go` +63/−24, `tree.go` +17/−1, `evaluate.go` +5/−2,
  `load.go` +3; `internal/doctor/checks.go` +5/−1;
  `internal/projection/registry.go` +2/−2.
- Test Go: +179 (`open_test.go` 127, doctor 25, `registry_test.go` 23, pack
  test 4).
- CUE: +16 / −42 (`examples/gcp/schema/gcp/policies.cue`).
- Docs: compliance policy design +8/−2, this log, plan.
