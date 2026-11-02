# GCP content pack — pudl-xf5.1

An installable GCP pack written in CUE: draft schemas for the core inventory,
`#CommandObserve` collectors (live and fixture), single-resource policies,
cross-resource Datalog checks, three check families, fixtures and a README.
Installed with `pudl example install gcp`. User docs:
`examples/gcp/README.md`.

## Packaging

`examples/gcp/` mirrors the `.pudl/` layout (`schema/...`, `populators/gcp/`)
and is embedded by `examples/gcp/bundle.go` (`//go:embed schema populators`).
`internal/examples.Install` gains a `gcp` case and `treeFiles`, which installs
every embedded file at its own path, so later packs need only a bundle and a
case. Go is needed only because examples ship inside the binary
(`go:embed`); it holds no GCP knowledge. `cmd/example.go` adds `gcp` to
`ValidArgs`, the help text and the next-step hint.

## Contents

- **Schemas** (package `gcp`, all `review: status: "draft"`): `#Network`,
  `#Subnetwork`, `#Firewall`, `#Route`, `#Router`, `#Address`, `#GKECluster`,
  `#SQLInstance`, `#Bucket`, `#RunService`, `#DNSManagedZone`, `#PubSubTopic`,
  `#PubSubSubscription`, `#KMSKeyRing`, `#KMSKey`. Identity is `selfLink` or
  the fully qualified `name`; buckets use their globally unique `name`; Cloud
  Run uses namespace (project number), location label and name, because its
  selfLink has no region; DNS zones use project and name.
- **Sensitive fields**: Cloud Run env values and creator/modifier
  annotations; GKE `nodeConfig.metadata`, `nodePools[*].config.metadata`,
  `masterAuth.password`/`clientKey`; Pub/Sub `pushConfig.pushEndpoint`.
- **Fact projection** only where checks need it: `gcp_subnet`, `gcp_router`,
  `gcp_router_nat_ip`, `gcp_address`, `gcp_gke_cluster`,
  `gcp_gke_secrets_key`, `gcp_kms_key`.
- **Collectors**: `gcp-<type>` (gcloud, fan-out over `_gcpProjects` by
  comprehension, `set: project`; KMS key rings over projects ×
  `_gcpKMSLocations`, keys over `_gcpKMSKeyRings`) and `gcp-fixture-<type>`
  (`cat` of the bundled fixture). Each declares a complete observation of
  `gcp/<type>` or `fixture/gcp/<type>`.
- **Policies** (rule, severity):
  - `#FirewallBestPractice`: `gcp-firewall-world-ingress` high.
  - `#AddressBestPractice`: `gcp-address-unused` low (external only).
  - `#BucketBestPractice`: `gcp-bucket-public-access-prevention` high,
    `gcp-bucket-uniform-access` medium (both `absent="violation"`).
  - `#SQLInstanceBestPractice`: `gcp-sql-backups` high (absent violation),
    `gcp-sql-pitr` medium (PITR flag or MySQL binary log; reported on
    `databaseVersion`), `gcp-sql-public-ip` medium (absent violation),
    `gcp-sql-world-authorized-network` critical.
  - `#KMSKeyBestPractice`: `gcp-kms-rotation` medium (ENCRYPT_DECRYPT keys
    only; reported on `purpose`).
  - `#GKEClusterBestPractice`: `gke-private-nodes` high (absent violation),
    `gke-control-plane-access` high (private endpoint or authorized networks;
    reported on `endpoint`), `gke-workload-identity` medium (absent
    violation).
- **Rules** (`schema/pudl/rules/gcp_pack.cue`): `gcp_policy_noncompliant_high`
  (high or critical, over the built-in `policy_noncompliant`),
  `gcp_nat_ip_not_in_use` (router × address),
  `gcp_gke_subnet_without_private_access` (cluster × subnet),
  `gcp_gke_secrets_key_unusable` (cluster × KMS key; disabled, destroyed or
  scheduled for destruction).
- **Families** (checks-only, evidence `model:<collector>`, `max_age: 1h`,
  `depends_on` their collectors): `gcp[-fixture]-network-hygiene-checks`,
  `-data-protection-checks`, `-gke-posture-checks`. Each has `policy-high`
  (fail) and `policy-any` (warn, over `policy_noncompliant`); network hygiene
  adds `nat-ip-not-in-use`; GKE posture adds
  `private-nodes-without-private-google-access` and `secrets-key-unusable`.
  Policy checks select only the family's own types, so a KMS rotation
  violation is not reported by GKE posture.
- **Fixtures**: 15 files of gcloud-shaped JSON for `example-project`, RFC 5737
  and RFC 1918 addresses, obviously fake tokens and scripts.

"Unused reserved addresses" is a policy, not a Datalog check: gcloud reports
it on the address itself (`status`), so it is single-resource.

## Tests

`cmd/gcp_pack_test.go` `TestGCPContentPack`: install (and an idempotent
reinstall), one `pudl check` of all 15 fixture collectors and 3 families
(exit 2), then:

| Family | Check | Verdict |
| --- | --- | --- |
| network hygiene | `policy-high` | fail, 1 (the firewall) |
| | `policy-any` (warn) | fail, 3 |
| | `nat-ip-not-in-use` | fail, 1 (`nat-ip-2`) |
| data protection | `policy-high` | fail, 4 (two buckets, two SQL instances) |
| | `policy-any` (warn) | fail, 10 |
| GKE posture | `policy-high` | fail, 1 |
| | `policy-any` (warn) | fail, 2 |
| | `private-nodes-without-private-google-access` | fail, 1 (`batch-private`) |
| | `secrets-key-unusable` | fail, 1 (`old-etcd-key`) |

`pudl policy evaluate --json` counts (compliant/noncompliant/unknown, all
provisional): Address 3/2/0, Bucket 1/2/0, Firewall 4/1/0, GKECluster 2/1/0,
KMSKey 4/1/0, SQLInstance 1/3/0. Every SQL rule appears in
`policy_violation`. Redacted values (a Cloud Run env token, GKE startup
scripts) are absent from listings; secret references remain. After the
noncompliant records are removed from the fixtures, the same check exits 0
with every check passing.

## Engine findings (reported, not fixed)

1. **A comprehension whose condition is incomplete in the bare policy drops
   every rule of the policy, silently.** `internal/policy/tree.go`
   `merge` ignores the error from `v.Fields(...)`. Repro (standalone policy on
   any type with `direction`): `direction?: _`,
   `sourceRanges?: _ @pudl(rule="r", severity="high")`,
   `if direction == "INGRESS" {sourceRanges?: [...!="0.0.0.0/0"]}`. Loaded
   `Rules` is `null`; a violation is reported as rule `sourceRanges[*]`,
   severity medium. Workaround in the pack: hidden fields with defaults
   (`_direction: *"" | string; if direction != _|_ {_direction: direction}`).
2. **Closed nested structs break references inside a policy.** "Evaluation
   ignores closedness" holds for reported errors, not for comprehensions: with
   `privateClusterConfig: {enablePrivateNodes: true, enablePrivateEndpoint?: _}`
   and a record whose `privateClusterConfig` has extra fields,
   `privateClusterConfig.enablePrivateEndpoint != _|_` is false, so a cluster
   with a private endpoint was reported noncompliant. Workaround: `...` in
   nested policy structs.
3. **Conditional absence is not expressible.** Absence is found from a tree
   built from the bare definition, so a requirement that depends on another
   field (PITR flag by engine, rotation by key purpose, private endpoint OR
   authorized networks) cannot report an absent field. Workaround: a conflict
   on a field every record carries (`databaseVersion`, `purpose`,
   `endpoint`), documented per policy.
4. **Policies are treated as family members by doctor and projection.**
   Standalone policies (as the design requires) without `identity_fields`
   make `pudl doctor` warn "Inconsistent identity_fields within schema
   families", and, when the type declares `facts`, every `pudl query` warns
   "projection disabled for <policy>: declares facts but no identity_fields"
   (facts are inherited along `base_schema`). Workaround: each pack policy
   repeats its type's `identity_fields`. Fix belongs in the doctor check and
   the projection registry (skip `schema_type: "policy"`).
5. **Checks-only models fail doctor validation.** Their persisted instance
   has `populate: {}`, which fails `#SystemModel`, so `pudl doctor --json`
   reports `ok: false` (`catalog` status `invalid`). Repro: a model with only
   `name` and `checks`, `pudl check` it, then `pudl doctor --json`. Not
   pack-specific.

Also: list addition (`a + b`) is rejected by the CUE version in use; the pack
uses `list.Concat`. A `populate` built from hidden fields stays ambiguous
across the retired arms unless written as `sm.#CommandObserve & {...}`.

## Public API

- `pudl example install gcp`.
- `internal/examples`: `Install(root, "gcp")`; `treeFiles(fs.FS)` (unexported).
- `github.com/chazu/pudl/examples/gcp`: `var Files embed.FS`.
- CUE: the schemas, policies, rules and models listed above.

## Line counts (against `d4e0408`, the sre-focus base)

- Production Go: +36 / −2 (net +34): `examples/gcp/bundle.go` 8,
  `internal/examples/install.go` +19/−1, `cmd/example.go` +9/−1.
- Test Go: +211 (`cmd/gcp_pack_test.go`).
- CUE: +836 (schemas 347, policies 162, rules 70, models 257).
- Fixtures: +836 lines of JSON. Docs: README 151, notes, cli-reference,
  help golden.

## Verification

`CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 mise exec -- make lint` (0
issues), `mise exec -- make check-docs check-skills` pass. No live GCP run was
made (no credentials); the README documents a one-project live run.
