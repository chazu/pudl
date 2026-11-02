# Identity edges: GCP service accounts and project IAM; AWS instance profiles and egress

## GCP

- `gcp.#ServiceAccount` (`examples/gcp/schema/gcp/iam.cue`): identity `email`,
  claimed by `name: projects/*/serviceAccounts/*`. Graph refs include
  `serviceAccount:<email>` (new `prepend` option), which is how IAM bindings
  name it. Collector `gcp-service-accounts`. A compute instance's `runs_as`
  now resolves to a collected account; the default compute account, which is
  not listed, stays a reference.
- `gcp.#ProjectIAMPolicy`: `gcloud projects get-iam-policy P` (one record per
  project; the collector sets project), claimed by its `bindings`. Fact
  `gcp_iam_binding` (each binding: role, each member, conditional). Rules:
  `iam_binding` member → policy node, with `role` (a list when a member holds
  several) and `conditional: "yes"`. Only project-level policy: inherited,
  conditional and deny policies are not evaluated.
- Exporter: `allUsers` is the same external "Anyone" node as `*`, and
  `allAuthenticatedUsers` is "Any signed-in Google account". Neither shows
  as "not in inventory".

## AWS

- `aws.#IAMInstanceProfile` (`iam list-instance-profiles`), collector
  `aws-iam-instance-profiles`. The instance fact gains `instance_profile`.
  Edges: `has_instance_profile` (instance → profile) and `profile_role`
  (profile → role).
- Security-group egress: `aws_sg_egress_group`, `aws_sg_egress_cidr` and
  `aws_sg_egress_cidr6`; edge `allows_egress_to` with protocol and port.

## Shared

- `graphspec.Value.Prepend` (`{path, prepend}`), documented in
  schema-authoring.
- UI vocabulary: `profile_role`, `allows_egress_to`, `gcp.iam.project_policy`
  and `aws.iam.instance_profile`; `has_instance_profile` now reads
  "instance runs with this instance profile".

## Verification

- Pack e2e tests: GCP `runs_as` resolves or stays a reference; bindings from
  a service account, a user reference and allUsers (Anyone) carry role and
  conditional properties. AWS: instance → profile → role; default egress
  allow-all with protocol -1 and no port. Plain-import inference of the new
  GCP types. Exporter tests for the public principals and `prepend`.
