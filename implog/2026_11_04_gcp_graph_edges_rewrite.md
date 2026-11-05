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

## Follow-up: absent fields (same day)

On a real catalog six of the new rules warned "all
rows have a null source or target": a plain path arg whose field is absent
is left out of the fact, so most routes still yielded a `gcp_route_next_hop`
fact with no `hop`, and the rule matched it with a null target. The
fixtures missed it because some rows always had the field.

- `internal/projection/compute.go`: an arg with `rewrite` whose field is
  absent (and has no default) now yields no fact, like a value no rule
  matches. Plain path args are unchanged.
- GCP pack: every new target arg is a rewrite (`gcp_bucket_key`,
  `gcp_subscription_push`, and the route next hops through the new
  `_gcpComputePathRewrite`).
- Tests: `TestRewriteWithoutAMatchYieldsNoFact` (absent field);
  `TestGraphExportGCPPack` counts facts per relation (fails without the fix:
  5 route facts instead of 1).

The behaviour change alone left existing catalogs' facts stale (a spec
that did not change kept its fingerprint, so `gcp_sql_key` was never
reprojected). The fingerprint of a rewritten arg now carries `AbsentNone`
(`internal/projection/registry.go`), so every catalog projected before the
change reprojects (`pudl doctor` reports it; `--fix --only projections` or
the next import/check/install applies it). Verified on the global catalog:
every affected resource reprojected, the `gcp_sql_key` warning gone.

`gcp_graph_topic_key` and `gcp_graph_subscription_dead_letter` (older rules
on plain path args) warned the same way on real data; their args
(`gcp_topic_key.key`, `gcp_subscription_dead_letter.topic`) are now rewrites
too, covered by the same fact-count test.

## Follow-up: service account policies name the account by email

On live data Cloud Asset Inventory names a
service account's policy resource
`//iam.googleapis.com/projects/P/serviceAccounts/EMAIL`, not by unique ID as
pudl-q4v.16 described; none matched. The rule now takes the last
segment (email, or a unique ID, which `#ServiceAccount` still declares as a
ref), and the fixture uses the email form. Also seen live: project
policies resolve to workspace `gcp.project` records through a `projects/<id>`
ref; KMS key policies stay references when the keys' location is outside
`_gcpKMSLocations`.
