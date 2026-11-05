# GCP pack: batch (c) — AlloyDB, Redis, Firestore and Cloud SQL backups (pudl-q4v.7)

Date: 2026-11-05

Third of the six batches of new GCP types in pudl-q4v.7 (design:
`docs/design/2026-11-03-gcp-cloud-asset-collectors.md` §5, §7). Batches
(d)–(f) remain.

## What was added

Four types collected from Cloud Asset Inventory (CAI) first, with a
per-project gcloud fallback, identity the full resource name:

| Collector | Schema | gcloud fallback |
| --- | --- | --- |
| `gcp-alloydb-clusters` | `gcp.#AlloyDBCluster` | `alloydb clusters list` |
| `gcp-alloydb-instances` | `gcp.#AlloyDBInstance` | `alloydb instances list` |
| `gcp-redis-instances` | `gcp.#RedisInstance` | `redis instances list --region=-` |
| `gcp-firestore-databases` | `gcp.#FirestoreDatabase` | `firestore databases list` |

Two through gcloud only, the newest `_gcpBackupHistory` (new live setting,
default 10) per Cloud SQL instance:

| Collector | Schema | Identity | Helper |
| --- | --- | --- | --- |
| `gcp-sql-backups` | `gcp.#SQLBackup` (Backup API, `sql#backup`) | `name` | `populators/gcp/sql-backups.sh` |
| `gcp-sql-backup-runs` | `gcp.#SQLBackupRun` (`sql#backupRun`) | `selfLink` | `populators/gcp/sql-backup-runs.sh` |

`sql-backup-runs.sh` lists the project's instances, then
`sql backups list --instance=I --limit=N` for each (a read replica has
none). `sql-backups.sh` takes the instances the project's backups name, so a
deleted instance's final backups are kept, then
`sql backups list --filter='instance="I"' --limit=N` for each. gcloud lists
both newest first.

Files: `schema/gcp/datastores.cue`, `schema/models/gcp_datastores.cue`,
`schema/models/gcp_sql_backups.cue`, `_gcpBackupHistory` in
`schema/models/gcp_pack.cue`.

### Facts (public relations)

- `gcp_alloydb_cluster(project, name, state, database_version,
  automated_backup, continuous_backup)`; `gcp_alloydb_cluster_network(network)`,
  `gcp_alloydb_cluster_key(key)` when set
- `gcp_alloydb_instance(project, name, cluster, type, public_ip, ssl_mode)`
- `gcp_redis_instance(project, name, tier, version, auth,
  transit_encryption)`; `gcp_redis_instance_network(network)`
- `gcp_firestore_database(project, name, type, location, pitr,
  delete_protection)`
- `gcp_sql_backup(project, instance, name, type, state, end_time)`
- `gcp_sql_backup_run(project, instance, id, type, status, end_time)`

Networks are in path form (`projects/P/global/networks/N`). No graph edges
yet.

### Redaction

AlloyDB's `initialUser.password` is declared sensitive (set at creation;
the API does not return it). Redis's AUTH string is not in the listing.
Nothing else in these types carries a secret.

## Shapes (`schema/models/gcp_datastores.cue`)

Live parity, values compared: CAI equals gcloud except

- AlloyDB: gcloud adds `updateTime` (moves without a configuration change)
  and `reconciling` → dropped;
- Redis: gcloud adds `availableMaintenanceVersions` and each server CA
  certificate's PEM → dropped (serial, fingerprint and dates stay);
- Firestore: CAI adds `deleteTime` (the zero time) and
  `enhancedTextSearchQueryMode`; gcloud adds `earliestVersionTime` (moves
  every minute), `etag`, `freeTier`, `keyPrefix`, `realtimeUpdatesMode`,
  `versionRetentionPeriod` → all dropped.

AlloyDB and Redis records also get `location`, read from the name.

## Verification

- Live, in a scratch workspace (nothing committed): the four CAI
  collectors stored what CAI holds; re-run through gcloud
  (`_gcpAssetScope: "off"`), no new records for any of them. Projects
  that refuse the list are failed inputs.
- Backups live: both collectors keep at most `_gcpBackupHistory` per
  instance, and list the same instances. Projects whose Cloud SQL list the
  caller may not read are failed inputs (Cloud SQL words this as "not
  authorized ... or it may not exist", also when its API is disabled, so
  such a project is never an empty input).
- Doctor clean on the fixture and live workspaces (no secret-scan findings).
- Fixtures (synthetic): two AlloyDB clusters (private services access
  with a customer-managed key, and Private Service Connect) with a
  primary, a read pool and a public-IP instance; an HA Redis instance with
  AUTH and TLS and a basic one without; two Firestore databases; backups
  and runs of two of the fixture SQL instances, an on-demand run, a failed
  run and a deleted instance's final backup. CAI and gcloud forms carry
  each source's extra fields.
- Tests: the four CAI types are in
  `TestGCPPackAssetSourcesStoreIdenticalRecords`; all six in the pack type
  lists; `cmd/gcp_pack_sql_backups_test.go` runs both helpers against a
  fake gcloud (history setting passed as `--limit`, replica without
  backups, deleted instance's backups kept). `CGO_ENABLED=0 go test ./...`
  passes.

## Deviation from the design

§5 planned for CAI to supply the Cloud SQL instance list. The helpers list
instances with gcloud instead, like the other per-project helpers; the
Backup helper takes them from the backups themselves, which also covers
deleted instances.

## Follow-ups

- Graph edges for these types (none yet, as for batches (a) and (b)).
