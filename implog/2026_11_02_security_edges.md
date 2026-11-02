# Security edges from real data

The security view had few edges on pack data because the packs projected no
security-group rules, firewall sources or IAM trust. Now they do.

## Projection: `one_or_many` (generic)

- `fieldpath.Path.LookupAllOneOrMany`: `[*]` over a non-list yields the
  value itself.
- `projection.RelationSpec.OneOrMany` from `{each, one_or_many: true,
  args}`. It applies to `each` and to every arg path in the relation. The
  fingerprint includes it with `omitempty`, so existing schemas keep their
  fingerprints and are not re-projected.
- Finding: the query layer returns booleans (stored or constant) as `1`.
  Rules match on `true` but emit strings.

## Exporter

- External endpoints: unresolved values that parse as CIDRs become
  `external.cidr` resources (`ext:cidr:<range>`; 0.0.0.0/0 and ::/0 are
  named "Anywhere"). `*` becomes `external.principal` "Anyone" (`ext:any:*`).
  Neither is ever shown as "not in inventory".
- `graph_edge_property(type, source, target, key, value)`: per-key values,
  deduplicated and sorted (numbers numerically), set on
  `Relationship.Properties`. Rows for edges no rule derived are reported.

## Packs

- AWS facts: `aws_sg_ingress_group`, `aws_sg_ingress_cidr`,
  `aws_sg_ingress_cidr6` (each `IpPermissions[*]`), `aws_role_trust` and
  `aws_role_trust_any` (`one_or_many`). Rules: `allows_ingress_from` with
  protocol and port properties, and `trust_allows_assume` (an ARN or account
  principal, or `*`) with `conditional: "yes"`. Service principals are not
  projected.
- GCP facts: `gcp_firewall_source` (range, direction, disabled) and
  `gcp_firewall_allow` (each allowed protocol and port). Rules: enabled
  INGRESS rules with an allowed entry give `allows_ingress_from` with port
  and protocol properties. Deny rules and disabled rules permit nothing.

## Layout

- `layout/isolatedPacking.ts`: three or more edge-less leaves under one
  parent are grid-packed in a placeholder box. Security view on the fixtures:
  5314×880 to 3792×1046, and to 1653×1494 with left-to-right, now its
  default.

## Verification

- Go: one_or_many tests (single and list shapes, unchanged default,
  fingerprint), external endpoint and property merge tests. The pack e2e
  test asserts SG→SG, SG→range, IPv6, the 22/tcp properties, a two-port edge,
  trust from `*` in both policy shapes, a conditional cross-account trust,
  no service principals, and GCP firewall ingress edges.
- Web: isolated packing test (grid, inside group, no overlaps, edges still
  routed). The fixture workspace was checked in Chromium.
