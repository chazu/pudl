# Findings: cataloging GCP resources with pudl

Observations from using pudl to catalog GCP resources (network, GKE, Cloud SQL,
Storage, Cloud Run, DNS, Pub/Sub, KMS) with user schemas in
`~/.pudl/schema/pudl/gcp/`. Workflow: inspect shape with `gcloud` -> write user
CUE schema -> `pudl import --schema`. The user is a GCP shop and does not use mu.

Status: addressed — verified against the source and implemented; see
`docs/design/2026-10-08-gcp-cataloging-ux.md` (decisions, review log,
corrections to the items below) and `docs/projection.md`. Original text follows.

Status (original): observations from one session. Items marked **unverified** were inferred
and need a check against the source before acting on them.

## What worked

- User schemas with `_pudl` metadata, open structs and `@pudl(binding=plain)`
  validated against live gcloud output with `cue vet -c -d`.
- Explicit `--schema` import of ndjson produced a `core.#Collection` with one
  item per record, with the item schema applied.
- `pudl show <id> --raw` plus `jq` was sufficient for verification.
- About 54 network records for one project, plus 8 further resource types, were
  imported with counts matching the source.

## Improvements, by priority

### 1. Item payloads are invisible to Datalog, checks and models (highest)

`pudl query` and `#Check` operate on facts and catalog metadata
(`catalog_entry` is join-only). They cannot read imported item fields, so
questions like "which firewalls allow 0.0.0.0/0?" or "which buckets lack public
access prevention?" cannot be asked. All verification fell back to `jq` over
`pudl show --raw`.

Proposal: project schema-declared fields into facts at import. `identity_fields`
and `tracked_fields` already exist in `_pudl`; they could define relations such
as `gcp.firewall(project, name, sourceRanges, ...)`. This would unlock checks,
drift and cross-resource relationships (GKE -> subnet, GKE -> KMS key,
SQL -> network, DNS zone -> network, PSC forwarding rule -> address).

**Unverified:** whether a custom-fact ingest path already exists. Read the fact
store and ingest code first.

### 2. Models cannot wrap a plain populate command

A `#SystemModel` populate arm is `#PluginObserve | #EweTarget`. `#EweTarget`
goes through `mu build`. A shell script that emits ndjson and calls
`pudl import` is the common case and cannot be expressed.

Proposal: a command arm (e.g. `#CommandTarget`): run argv, ingest ndjson output
with a declared schema, with an `inputs` list for per-project fan-out (replacing
a bash loop over projects).

Also: `pudl model new <name> --populate plugin:k8s` scaffolds a literal
`plugin: "k8s"` arm, which is meaningless for non-k8s targets. The scaffold
should offer the command arm.

### 3. Schema selection and import ergonomics

- Without `--schema`, ndjson becomes `core.#Collection`; inference did not pick
  user schemas. Allow a match hint (e.g. a `resource_type` field or a match rule
  in `_pudl`).
- `pudl list --schema 'pudl/gcp.#Route'` also matches `#Router` (prefix
  overlap). Schema filters should match exactly, with prefix matching opt-in.
  Workaround used: Route count = total - Router.
- gcloud often omits `project` from records. Required injecting it with
  `jq '.[]+{project:$p}'`. Suggest `pudl import --inject key=value`.

### 4. Re-import and identity behaviour

- Re-running an import did not appear to create duplicates, but only network and
  subnet counts were checked. **Unverified** for other types, and for how
  changed resources are recorded.
- Dotted `identity_fields` are used by examples/kubernetes.cue. A quoted key
  containing dots (`metadata.labels."cloud.googleapis.com/location"`) was used
  for Cloud Run and is **untested**. `pudl schema` validation could warn when an
  identity path resolves to nothing on sample data.

### 5. Sensitive data handling

Nothing flags or redacts secrets at import. The data imported here contained no
credentials (Cloud Run secrets were `valueFrom` refs; certs were public CAs),
but it did include internal IPs, firewall sources, authorized networks and user
emails (Cloud Run annotations). Other resources (Cloud Run/Functions env vars,
Compute instance metadata and startup scripts) can hold literals.

Proposal: a schema-level `@pudl(sensitive)` attribute that redacts or hashes a
field on import.

### 6. Smaller items

- Validation needs `cue vet -c -d "#X" <all schema files> file.ndjson` with a
  hand-built file list and ndjson input. Suggest `pudl schema validate <schema>
  <file>`.
- `pudl show --raw` prints a "RAW DATA" header before the payload; scripts must
  strip it. Suggest `--payload-only`.
- `pudl import` takes one file; accept a directory/glob with a schema mapping.
- `pudl list --json` is an object (`entries`, `total_matched`, ...), not an
  array; document it.
- Per-location resources (KMS key rings) require iterating locations; this is a
  populator concern, but a fan-out `inputs` facility (item 2) would cover it.

## Suggested order

1. Project schema fields into facts (item 1).
2. Command populate arm (item 2).
3. Exact schema match and `--inject` (item 3).
4. Sensitive-field redaction (item 5).

## Model ideas blocked by items 1 and 2

- `gcp-network-hygiene`: open-world ALLOW firewalls (check `denied` as well:
  GKE exkubelet is a DENY 0.0.0.0/0), unused RESERVED addresses, NAT IPs not
  IN_USE.
- `gcp-data-protection`: buckets without public access prevention, SQL without
  backups/PITR or with public IP, KMS keys without rotation or with DESTROYED
  primary, GKE encryption key present and enabled.
- `gke-posture`: private nodes/endpoint, authorized networks, workload identity,
  network policy, version skew between paired clusters.
- `gcp-dependency-graph`: resolve cross-resource references and detect dangling
  ones.
- `gcp-failover-parity`: desired-state parity of doppler-primary/failover.
