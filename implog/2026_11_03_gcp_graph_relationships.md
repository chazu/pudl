# GCP pack: more graph relationships

**Date:** 2026-11-03

## Work done

Reviewed `pudl graph vocab` and the edges the fixtures actually produce
(48 edges; Cloud Run services, buckets and key rings had none), then added
eight relationships, all with built-in edge types, so the vocabulary is
unchanged and still reports no problems. The fixture graph now has 63 edges.

| Edge | From → to | New fact |
|---|---|---|
| `peers_with` (+`state`) | network → peer network | `gcp_network_peering` |
| `applies_to` (+`matched_service_account`) | firewall rule → instances running as a target service account | `gcp_firewall_target_sa` |
| `attached_to` | address → its users | `gcp_address_user` |
| `allows_ingress_from` | public Cloud SQL instance → authorized network | `gcp_sql_authorized_network` |
| `allows_ingress_from` | GKE control plane → enforced authorized network | `gcp_gke_authorized_network` |
| `runs_as` | Cloud Run service → service account | `gcp_run_service_sa` |
| `encrypted_by` | Pub/Sub topic → KMS key | `gcp_topic_key` |
| `publishes_to` | subscription → dead-letter topic | `gcp_subscription_dead_letter` |

Fixtures: both networks peer with each other, and a new firewall rule
(`allow-health-checks-orders`) targets the orders-api service account; the
CAI fixtures were regenerated from them.

Considered and left out:

- KMS key → key ring (`part_of`): projection cannot derive the ring from the
  key name, and adding a derived field would change stored records.
- GKE node pools → service account: the common value is the literal
  `default`, and rules cannot filter it out (no negation).
- Route next hops, push subscriptions → Cloud Run, bucket and SQL CMEK keys:
  not in the fixtures; candidates for later.

Known overlap: GCP lists a Cloud NAT router among its address's users, so
that pair shows both `nat_ip` (router → address) and `attached_to`.

## Public API

New facts on `gcp.#Network`, `#Firewall`, `#Address`, `#SQLInstance`,
`#GKECluster`, `#RunService`, `#PubSubTopic` and `#PubSubSubscription` (table
above), and ten rules in `schema/pudl/rules/gcp_graph.cue`. README gains a
Graph section listing every relationship.

## Tests

`TestGraphExportGCPPack` asserts each new edge, its properties, the
unresolved forwarding-rule reference, and non-edges (an instance running as
another account; a private SQL instance). `TestGCPContentPack` counts
updated for the new firewall rule (FirewallBestPractice compliant 5;
6 firewalls depend on prod-vpc).
