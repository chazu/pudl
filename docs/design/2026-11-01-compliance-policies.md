# Compliance policies: schema specializations evaluated per resource

**Date:** 2026-11-01

**Status:** Implemented (2026-11-02). Tracked by beads epic `pudl-xf5.8`
(steps `pudl-xf5.8.1`, `.2`, `.3`, `.4`, `.7` and `.9`). The authoring guide
is [Compliance Policies](../schema-authoring.md#compliance-policies).

## Decision

Separate a resource's **type** from its **compliance**.

- A **type schema** (`schema_type: "base"` or `"custom"`) says what a resource
  is, for example `aws.#S3Bucket`. Classification assigns exactly one type to
  each record.
- A **policy schema** (`schema_type: "policy"`) is a CUE specialization of a
  type, for example `aws.#S3BucketBestPractice: #S3Bucket & {...}`. It says what
  a good instance looks like. Policies are never assigned as a record's schema.
  Instead, PUDL evaluates every applicable policy against every resource and
  records a result per resource and policy, with named violations.
- **Validated** means a person or agent has reviewed a type schema and blessed
  it as a complete representation of the resource. It is review metadata on the
  schema. The existing record-level `validation_status` ("this record satisfied
  the explicitly requested schema") is unchanged.

Policies cover rules about a single resource. Datalog checks remain for rules
that span resources, such as "a GKE cluster whose KMS key is disabled".

## Problem

`docs/schema-authoring.md` already documents `"policy"` as "compliance
checking", built through `base_schema`. The Go code does not treat it
specially (`internal/validator/validation_result.go:39` only labels it), so:

- **Compliance means classification.** Policy schemas compete in inference with
  type schemas. A compliant bucket is classified as the policy; a non-compliant
  one silently falls back to `#S3Bucket`.
- **One policy per resource.** A record has one assigned schema, so independent
  policies (encryption, tagging, PCI) cannot all apply.
- **No violation detail.** Field-level validation errors are not persisted (see
  the 2026-11-01 conformance review), so nobody can see why a bucket is not
  compliant.
- **Explicit imports are wrong for inventory.** `--schema aws.#S3Bucket` never
  evaluates policies. `--schema aws.#S3BucketBestPractice` rejects the whole
  import if any bucket is non-compliant.

Identity is already safe to change: `resource_id` is namespaced by the root of
the inheritance family (`internal/identity/resource_id.go`), so moving a record
from a policy leaf to its type does not change its identity or history.

## Authoring

```cue
#S3Bucket: {
    _pudl: {
        schema_type:     "base"
        resource_type:   "aws.s3.bucket"
        identity_fields: ["Name"]
        review: {status: "validated", by: "sre-platform", at: "2026-11-01"}
    }
    Name: string
    PublicAccessBlock?: {...}
    ServerSideEncryption?: {...}
    ...
}

// Policies are written standalone and linked to their type by base_schema.
// `#S3Bucket & {...}` does not load: _pudl.schema_type conflicts ("base" vs
// "policy"), and the closed definition rejects the policy's added fields.
// The engine assembles the chain from base_schema itself.
#S3BucketBestPractice: {
    _pudl: {
        schema_type: "policy"
        base_schema: "aws.#S3Bucket"
        description: "Baseline S3 hardening"
    }
    PublicAccessBlock: {
        BlockPublicAcls: true @pudl(rule="s3-block-public-acls", severity="high")
        RestrictPublicBuckets: true @pudl(rule="s3-restrict-public", severity="high")
    } @pudl(absent="violation")
    ServerSideEncryption: {
        Algorithm: "aws:kms" | "AES256" @pudl(rule="s3-encryption", severity="medium")
    } @pudl(absent="violation")
}

// A policy may extend another policy. The chain must end at a type.
#S3BucketPCI: {
    _pudl: {schema_type: "policy", base_schema: "aws.#S3BucketBestPractice"}
    Versioning: Status: "Enabled" @pudl(rule="s3-versioning", severity="medium")
}
```

- **Applicability.** A policy applies to a resource when walking the policy's
  `base_schema` chain reaches the resource's assigned type or one of that
  type's ancestors. Several policies may apply to one resource.
- **Rule attributes.** `@pudl(rule=..., severity=low|medium|high|critical,
  doc=...)` names a constraint. A violation reports the nearest annotated rule
  at or above the failing path. Unannotated constraints report their path as the
  rule name. Annotations are read from the bare policy, so a field annotated
  only inside a comprehension whose condition reads record data (`if direction
  == "INGRESS" {...}`) cannot be read, and the policy fails to load. Annotate
  the field outside the comprehension (`sourceRanges?: _ @pudl(rule=...)`) and
  constrain it inside.
- **Absent fields.** A field the policy constrains but the record lacks is
  `absent`. By default an absent field makes the result `unknown`, because the
  collector may not have gathered it. `@pudl(absent="violation")` makes absence
  a violation; use it where absence means the setting is off. A field that is
  required (non-optional) in a **validated** type schema is always treated as
  collected, so its absence is a violation.
- **Extra fields.** Policies constrain only the fields they name. Evaluation
  ignores closedness at every depth, so extra fields never cause a violation,
  and extra fields under a nested policy struct do not break references into
  it. Nested policy structs need no `...`.
- **Sensitive paths.** Values on `sensitive_fields` paths are redacted before
  storage, so a policy cannot evaluate them. A policy that constrains a path the
  type declares sensitive fails to load.

## Evaluation

- **What it produces.** For each pair of resource and applicable policy:
  `compliant`, `noncompliant` (at least one violation), or `unknown` (no
  violations, but some absent fields). Each violation records the rule,
  severity, path and kind (`conflict` or `absent`). Values are never stored.
- **When it runs.** Policies are evaluated after every import and every
  `#CommandObserve` ingest, over the records being ingested, in the same
  transaction that projects facts. `pudl policy evaluate [POLICY...]`
  re-evaluates stored current resources without re-importing; payloads are
  retained.
- **Policy versions.** Each result records the digest of the policy's effective
  CUE. Results whose digest differs from the loaded policy are stale. `pudl
  policy list` and `pudl doctor` report stale results, and `pudl policy
  evaluate` refreshes them. History can then tell "the resource changed" from
  "the policy got stricter".
- **Draft types.** Results against a type schema that is not `validated` are
  marked `provisional` in output. They are still computed.

## Storage

Results are bitemporal facts, so they keep history and are queryable in
Datalog, and the governance work (waivers, finding changes over time,
ownership) applies to them unchanged.

- `policy_status(resource, policy, type, status, policy_digest)`
- `policy_violation(resource, policy, rule, severity, path, kind)`

Facts for one resource and policy share a source (`policy:<policy>:<resource>`),
which is reconciled on each evaluation, the same way fact projection reconciles
`database.ProjectionSource`. A resource that disappears from a complete
observation scope has its policy facts closed, as projection does. The names
`policy_status` and `policy_violation` are reserved, so `AddFact` rejects them.

## Commands

| Command | Purpose |
| --- | --- |
| `pudl policy list` | Every loaded policy: name, the type it applies to, the policy it extends, rule count, the type's review status, last evaluated, compliant/noncompliant/unknown/stale counts. `--type T` filters. `--json` follows the versioned result contract. |
| `pudl policy show POLICY` | Rules with severity and doc, the inheritance chain, counts per rule, and the resources failing most. |
| `pudl policy evaluate [POLICY...]` | Re-evaluate stored current resources. `--dry-run` reports what would change. `--what-if FILE` evaluates a draft policy file without storing results. |
| `pudl show ID` | A compliance section: each applicable policy, its status and its violations. |
| `pudl schema list` | Adds each schema's review status (`draft`/`validated`, by, at). |

## Classification changes

- **Inference.** Policy schemas leave the inference candidate set and the
  `base_schema` fallback chain. Classification only ever assigns types.
- **Explicit imports.** `--schema` and a model's `schema:` must name a type.
  Naming a policy is an error that names the policy's type and suggests
  `pudl policy evaluate`.
- **Existing data.** Entries currently assigned to a policy leaf are moved to
  the nearest type in their chain. Their `resource_id` does not change. This is
  a `pudl doctor --fix` repair (`pudl-4fx.2`), followed by an evaluation that
  restores their compliance as policy facts.

## Relationship to other work

- **Checks.** `#Check` does not change. PUDL ships a built-in Datalog rule,
  `policy_noncompliant(resource, policy, severity)`, which ordinary checks
  query. Waivers (`pudl-xf5.4`) key on (policy, rule, resource).
- **Content packs.** The GCP and AWS packs (`pudl-xf5.1`, `pudl-xf5.2`) ship
  best-practice rules as policies where they concern one resource, and as
  relationship rules and checks (`pudl-xf5.7`) where they span resources.
- **Conformance.** Persisting field-level differences for records that fail
  their *type* schema is a separate concern, and uses the same violation
  extraction.

## Delivery

- `pudl-xf5.8.1`: exclude policies from classification. Error on `--schema`
  naming a policy. Add the `doctor --fix` repair for existing policy-leaf
  entries.
- `pudl-xf5.8.7`: schema review status (`_pudl.review`) and its display in
  `pudl schema list`.
- `pudl-xf5.8.2`: the evaluation engine (`internal/policy`): finding applicable
  policies, open unification, violation and rule extraction, absent-field
  semantics, the sensitive-path load error and policy digests. Pure and unit
  tested.
- `pudl-xf5.8.3`: persist results as facts at ingest. Add `pudl policy
  evaluate` with `--what-if`, the built-in `policy_noncompliant` rule, and
  stale detection.
- `pudl-xf5.8.4`: `pudl policy list`, `pudl policy show`, and the compliance
  section in `pudl show`, with JSON output.
- `pudl-xf5.8.9`: rewrite the policy section of `docs/schema-authoring.md`,
  and update the embedded skills and the CLI reference. Convert the GCP
  example's single-resource firewall rule into a policy, which removes its
  `rules.cue` and projection metadata.

Tickets `.8.5`, `.8.6` and `.8.8` were folded into `.8.3` and `.8.4` on
2026-11-01 to avoid bloat. Every implementation log reports the net
production-line delta.

## Resolved questions

Both were decided on 2026-11-01.

1. **Policy on policy.** A result for an extending policy (`#S3BucketPCI`)
   reports every rule, including those inherited from `#S3BucketBestPractice`,
   so each result stands alone. Each violation names the policy that declared
   its rule.
2. **Severity of the whole result.** A status carries the highest severity
   among its violations, used for sorting and check thresholds. It is derived
   when read, not stored.
