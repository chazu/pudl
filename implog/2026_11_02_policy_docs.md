# Compliance policy documentation; gcp-network-hygiene as a policy — pudl-xf5.8.9

Delivery step 9 of `docs/design/2026-11-01-compliance-policies.md`, whose
status is now Implemented.

## Documentation

`docs/schema-authoring.md` has one **Compliance Policies** section, which
replaces the scattered policy notes and the broken examples:

- type versus policy; applicability along the `base_schema` chain;
- standalone policies linked by `base_schema` (and why `#Type & {...}` does
  not load), with a type, a best-practice policy and an extending policy;
- rule attributes (`rule`, `severity`, `doc`, `absent`) and their load errors;
  nearest-rule attribution and path-as-rule for unannotated constraints;
- absent fields: `unknown` by default, a violation with `absent="violation"`
  or when required in a `validated` type; status derivation; provisional
  results against draft types;
- extending policies and inherited-rule reporting (`declared_by`);
- the sensitive-path load error;
- CUE gotchas: no short-circuit over absent fields and the `list.Contains`
  idiom; annotations outside comprehensions; conditional absence and the
  anchor workaround;
- known engine limitations;
- workflow: `pudl policy list`/`show`/`evaluate`, `--dry-run`, `--what-if`,
  the `pudl show` compliance section, the result relations, the built-in
  `policy_noncompliant` rule in checks, stale digests, and the
  `policy-results` and `policy-leaves` doctor repairs.

A new `review` subsection documents `_pudl.review`. The `#CompliantInstance:
#Instance & {...}` and `#AdminUser: #User & {...}` examples, neither of which
loaded, are replaced (the latter by a standalone `#UserBestPractice`). The
`policy-leaves` note moved from "After Adding or Changing Schemas" into the
section.

Every CUE example was loaded into a scratch workspace and evaluated against
sample records: the S3 type, best practice and PCI extension (a missing
`PublicAccessBlock` reports both rules as absent violations, declared by the
best practice, under both policies); the `list.Contains` policy; the anchor
policy (one violation at `Website`, while the plain conditional
`if Website != _|_ {Logging: _}` reported every bucket compliant); the
`--what-if` draft; and the load errors quoted for a comprehension-only
annotation and a sensitive path. A `||` condition over an absent field was
confirmed to apply nothing.

Also updated: the `pudl-core` skill (a Compliance policies subsection; the
`policy-results` repair in the doctor line), `docs/gcp-network-hygiene.md`,
and the `pudl example install` help (CLI reference and help golden
regenerated). `pudl guide` and `pudl prime` do not describe policies and are
unchanged.

## gcp-network-hygiene example

The Datalog rule `gcp_internet_ingress_allow` over three projected relations
becomes the policy `pudl/gcphygiene.#FirewallNoInternetIngress` in
`schema.cue`, rule `internet-ingress-allow` (high):

```cue
if allowed != _|_ if !list.Contains([if direction != _|_ {direction}], "EGRESS") if !list.Contains([if disabled != _|_ {disabled}], true) {
	sourceRanges?: [...!="0.0.0.0/0"]
}
```

An absent `direction` counts as INGRESS and an absent `disabled` as enabled,
as the projection defaults did. `rules.cue` and the schema's `facts` block
are deleted (nothing else used them); the install map and the embed list drop
`rules.cue`. The model's check queries `policy_noncompliant`.

Verdicts are unchanged: baseline fails with one finding (`public-ssh`;
`deny-world` and `private-web` are compliant), the corrected fixture passes,
and so does the saved scope check. A witness now carries `resource`, `policy`
and `severity` instead of the entry ID, protocol and port; the walkthrough
takes the record ID from `pudl list` and shows the violation with `pudl
policy show` and `pudl show`.

Example size: 94 → 84 lines (`rules.cue` −12, `schema.cue` 30 → 32).

## Tests

`TestGCPNetworkHygieneWorkflow` asserts the witness's policy and severity
instead of evidence IDs, and saves its check with `--check
policy_noncompliant`; counts, outcomes, snapshot comparison, history and the
report are asserted as before.

`CGO_ENABLED=0 go test ./...` passes; `CGO_ENABLED=0 mise exec -- make lint`:
0 issues; `make check-docs check-skills test-git-walkthrough` pass.

## Public API

None changed.

## Line delta (`git diff --numstat`)

- Production Go: +4 / −4 (`cmd/example.go` help 3/2,
  `internal/examples/install.go` 1/1, `bundle.go` 1/1).
- Test Go: +3 / −2.
- CUE: +16 / −26 (`schema.cue` 15/13, `model.cue` 1/1, `rules.cue` −12).
- Docs: `schema-authoring.md` +222/−28 (incl. a waiver pointer added after merging the waivers work), `gcp-network-hygiene.md` +24/−13,
  design +3/−2, skill +18/−1 (twice, embedded copy), plan, this log.
- Generated: `cli-reference.md` +3/−2, `help.golden.json` +1/−1.
