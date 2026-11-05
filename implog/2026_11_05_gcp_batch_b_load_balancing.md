# GCP pack: batch (b) — load balancing, Cloud Armor and DNS record sets (pudl-q4v.7)

Date: 2026-11-05

Second of the six batches of new GCP types in pudl-q4v.7 (design:
`docs/design/2026-11-03-gcp-cloud-asset-collectors.md` §5, §7). Batches
(c)–(f) remain.

## What was added

Eleven compute types, each collected from Cloud Asset Inventory (CAI) first
with a per-project gcloud fallback, identity `selfLink`:

| Collector | Schema | gcloud fallback |
| --- | --- | --- |
| `gcp-forwarding-rules` | `gcp.#ForwardingRule` | `compute forwarding-rules list` |
| `gcp-target-http-proxies` | `gcp.#TargetHTTPProxy` | `compute target-http-proxies list` |
| `gcp-target-https-proxies` | `gcp.#TargetHTTPSProxy` | `compute target-https-proxies list` |
| `gcp-url-maps` | `gcp.#URLMap` | `compute url-maps list` |
| `gcp-backend-services` | `gcp.#BackendService` | `compute backend-services list` |
| `gcp-healthchecks` | `gcp.#HealthCheck` | `compute health-checks list` |
| `gcp-http-healthchecks` | `gcp.#HTTPHealthCheck` (legacy) | `compute http-health-checks list` |
| `gcp-network-endpoint-groups` | `gcp.#NetworkEndpointGroup` | `compute network-endpoint-groups list` |
| `gcp-ssl-certificates` | `gcp.#SSLCertificate` | `compute ssl-certificates list` |
| `gcp-ssl-policies` | `gcp.#SSLPolicy` | `compute ssl-policies list` |
| `gcp-security-policies` | `gcp.#SecurityPolicy` (Cloud Armor) | `compute security-policies list` |

And `gcp-dns-record-sets` (`gcp.#DNSRecordSet`, identity project, zone,
name, type), gcloud only through `populators/gcp/dns-record-sets.sh`.

Health check collectors are named `healthchecks`, not `health-checks`: a
model name ending in `-checks` reads as a check family (`gcp-*-checks`).

### Facts (public relations)

References are compute paths (`projects/P/global/...`); a reference field
that is absent yields no fact.

- `gcp_forwarding_rule(project, name, ip, protocol, scheme, ports)`;
  `gcp_forwarding_rule_target(target)`, `gcp_forwarding_rule_backend(backend)`,
  `gcp_forwarding_rule_network(network)`
- `gcp_target_proxy_url_map(url_map)` (HTTP and HTTPS);
  `gcp_target_proxy_certificate(certificate)`,
  `gcp_target_proxy_ssl_policy(ssl_policy)`
- `gcp_url_map_default_service(service)`; `gcp_url_map_service(service)` per
  path matcher
- `gcp_backend_service(project, name, scheme, protocol, iap, logging)`;
  `gcp_backend_service_backend(group)`,
  `gcp_backend_service_health_check(health_check)`,
  `gcp_backend_service_security_policy(policy)`
- `gcp_health_check(project, name, type)`, `gcp_http_health_check(project, name)`
- `gcp_neg(project, name, type, run_service)`, `gcp_neg_network(network)`
- `gcp_ssl_certificate(project, name, type, expires)`,
  `gcp_ssl_certificate_domain(domain)`
- `gcp_ssl_policy(project, name, min_tls, profile)`
- `gcp_security_policy(project, name, type)`,
  `gcp_security_policy_rule(priority, action, preview)` per rule
- `gcp_dns_record(project, zone, name, type, data)` per record value

No graph edges yet (pudl-q4v.22).

### Redaction

Request header values a load balancer adds can carry credentials for the
backend: `customRequestHeaders[*]` and `iap.oauth2ClientSecret` on backend
services, every URL map `headerAction.requestHeadersToAdd[*].headerValue`
(top level, path matchers, route rules, weighted backends), and Cloud Armor
rules' header values. Certificates are public; DNS records are what a zone
publishes.

## Shapes (`schema/models/gcp_loadbalancing.cue`)

Live parity (field names only): CAI equals gcloud except `kind` and
`fingerprint` (as for other compute types) and:

- backend services: `usedBy` in another order → sorted by reference;
- forwarding rules: gcloud adds `attachedExtensions` → dropped;
- health checks: CAI can spell a region URL `compute/v2` → `compute/v1`;
- Cloud Armor policies: gcloud adds each rule's `kind` → dropped.

DNS record sets (`schema/models/gcp_dns_records.cue`): CAI holds no record
set of a project's managed zones, only those of GKE's cluster-scoped DNS
zones (pod and service records), which are not cataloged.

## Verification

- Live, in a scratch workspace (nothing committed): the eleven CAI
  collectors stored what the census search returned. Re-run through gcloud
  (`_gcpAssetScope: "off"`): no new records for any of them. Projects that
  refuse a global-only list are failed inputs; aggregated lists hide the
  refusal (pudl-zbj).
- `gcp-dns-record-sets` live: projects refusing `dns.managedZones.list`
  are failed inputs; the others' records stored.
- Doctor clean on both the fixture and the live workspace (no secret-scan
  findings on DNS data).
- Fixtures (synthetic): an external HTTPS load balancer (forwarding rules,
  proxies, URL maps with a redirect, backend services on a zonal and a
  serverless NEG, managed and self-managed certificates, SSL policy, Cloud
  Armor policy) and an internal TCP load balancer, in CAI and gcloud form
  with the differences above; DNS records for the existing zones.
- Tests: the eleven CAI types are in `TestGCPPackAssetSourcesStoreIdenticalRecords`;
  all twelve in the pack type lists (`gcp_pack_test.go`,
  `gcp_pack_inference_test.go`). `CGO_ENABLED=0 go test ./...` passes.

## Found on the way

- pudl-zbj (P1): gcloud's aggregated list commands exit 0 with only a
  stderr warning when a project refuses the list, so the project looks
  empty and its resources are marked gone. Affects existing collectors too
  (addresses, subnets, routers, instances).

## Follow-ups

- pudl-q4v.22: graph edges for the load balancing chain.
- pudl-zbj: the aggregated-list permission bug.
