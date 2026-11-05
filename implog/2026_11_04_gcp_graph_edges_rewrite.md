# GCP graph: eight more edges, and fact rewrites (pudl-q4v.11 to .17, .19)

**Date:** 2026-11-04

## Work done

### Product: `rewrite` on fact args

A `_pudl.facts` arg `{path, ...}` takes `rewrite: [{match, replace}, ...]`:
the first rule whose RE2 `match` matches the (string) value replaces it with
`replace` expanded over the submatches (`$1`, `${name}`; Go
`regexp.Expand`). A value no rule matches, or a non-string, yields no fact
for its row; for a `[*]` arg only that value is skipped. `exists` args take
no rewrite. Setting `rewrite` changes the schema's projection fingerprint
(facts are reprojected); specs without it keep their fingerprints.

- `internal/projection/rewrite.go` (new): `RewriteRule`, parsing, `ArgSpec.rewrite`.
- `internal/projection/spec.go`: `ArgSpec.Rewrite`, parsing and validation.
- `internal/projection/compute.go`: applied after `trim_prefix`; unmatched
  values drop their row (or wildcard value).
- `internal/projection/registry.go`: fingerprint includes rules (omitempty).
- `internal/projection/rewrite_test.go`, `docs/projection.md`.

### GCP pack edges

| Issue | Edge | Fact (schema) | Derivation |
|---|---|---|---|
| q4v.11 | key `in_key_ring` ring (new type) | `gcp_kms_key_ring` (`#KMSKey`) | name minus `/cryptoKeys/K` |
| q4v.12 | GKE cluster `runs_as` account, `node_pool` property | `gcp_gke_node_pool_sa` (`#GKECluster`, each node pool) | email only; `default` yields nothing |
| q4v.13 | route `routes_via` next hop | `gcp_route_next_hop`, `_vpn`, `_ilb` (`#Route`) | instance / VPN tunnel by trimmed URL; ILB only in URL form |
| q4v.14 | push subscription `pushes_to` endpoint (new type) | `gcp_subscription_push` (`#PubSubSubscription`) | `pushConfig.pushEndpointOrigin`; `#RunService` refs `status.url` |
| q4v.15 | bucket, Cloud SQL `encrypted_by` key | `gcp_bucket_key` (`default_kms_key`), `gcp_sql_key` (`diskEncryptionConfiguration.kmsKeyName`, version suffix dropped) | |
| q4v.16 | IAM policy `grants_on` resource | `gcp_iam_policy_resource` (`#IAMPolicy`) | bucket name; compute path; SA unique ID (`#ServiceAccount` refs `uniqueId`); `projects/P`; pubsub/KMS `projects/...` |
| q4v.17 | SA key `key_of` account (new type) | `gcp_service_account_key_account` (`#ServiceAccountKey`) | email from the key name |
| q4v.19 | Cloud Run service `runs_image` image (new type) | `gcp_run_service_image` (`#RunService`) | tag/digest dropped; gcr.io, us./eu./asia.gcr.io mapped to their Artifact Registry repositories |

New pack edge types (`gcp_graph.cue`): `in_key_ring` (containment),
`key_of` (iam), `pushes_to` (dependency), `runs_image` (dependency).

**Push endpoint origin.** The endpoint stays redacted (it can carry a
token). A new subscription shape (`_gcpShapes."pubsub-subscriptions"`,
`gcp_assets.cue`) adds `pushConfig.pushEndpointOrigin`, scheme and host
without user info, path or query. Push subscriptions get one new record
version on the next collection. Verified: no stored file contains the
fixture token.

### Fixtures

GKE node pools carry `config.serviceAccount` (gcloud and CAI copies); three
routes with instance, VPN tunnel and ILB next hops (both copies); a push
subscription to the Cloud Run service; a CMEK bucket and Cloud SQL
instance; the `orders-api` image the Run fixture references.

### Not verified live

No bucket or Cloud SQL instance in the surveyed organization uses CMEK, so
`default_kms_key` (gcloud storage) and `diskEncryptionConfiguration.kmsKeyName`
(gcloud sql) are gcloud's documented keys, not observed ones.

### Not done

- The pack collects no project records, so a project policy's `grants_on`
  target stays a reference unless a workspace schema for `gcp.project`
  declares the ref `projects/<id>`.
- Key-age checks for service account keys (mentioned in q4v.17) are not
  added.

## Public API

No Go API change. Schema surface: fact arg option `rewrite`; GCP facts and
edge types above.
