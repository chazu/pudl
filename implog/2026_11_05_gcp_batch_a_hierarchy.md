# GCP pack: batch (a) — hierarchy, policies and enabled services (pudl-q4v.7)

Date: 2026-11-05

First of the six batches of new GCP types in pudl-q4v.7 (design:
`docs/design/2026-11-03-gcp-cloud-asset-collectors.md` §5, §7). Batches
(b)–(f) remain; the issue stays open.

## What was added

Six types, each collected from Cloud Asset Inventory (CAI) first with a
gcloud fallback:

| Collector | Schema | CAI asset type | Identity | gcloud fallback |
| --- | --- | --- | --- | --- |
| `gcp-organizations` | `gcp.#Organization` | `cloudresourcemanager.googleapis.com/Organization` | `name` | `organizations describe` per organization |
| `gcp-folders` | `gcp.#Folder` | `cloudresourcemanager.googleapis.com/Folder` | `name` | `folders.sh` (breadth-first walk) |
| `gcp-org-policies` | `gcp.#OrgPolicy` | `orgpolicy.googleapis.com/Policy` | `name` | `org-policies.sh` (org, folders, projects; describe each) |
| `gcp-firewall-policies` | `gcp.#FirewallPolicy` | `compute.googleapis.com/FirewallPolicy` | `selfLink` | `firewall-policies.sh` (org and each folder) |
| `gcp-binauthz-policies` | `gcp.#BinaryAuthorizationPolicy` | `binaryauthorization.googleapis.com/Policy` | `project` | `binauthz-policy.sh` (drops never-set defaults) |
| `gcp-services` | `gcp.#EnabledService` | `serviceusage.googleapis.com/Service` | `project`, `name` | `services list --enabled` |

Each has a `gcp-fixture-*` twin reading synthetic fixtures
(`populators/gcp/<type>.json`, `populators/gcp/cai/<type>.json`).

### Facts (public relations)

- `gcp_organization(name, display_name)`
- `gcp_folder(name, display_name, parent, state)`
- `gcp_org_policy(resource, constraint, reset, inherit)`;
  `gcp_org_policy_rule(enforce, allow_all, deny_all, condition)` per rule
- `gcp_firewall_policy(parent)`; `gcp_firewall_policy_association(target)`
  per association; `gcp_firewall_policy_rule(priority, direction, action,
  disabled, src_range)` per rule and source range
- `gcp_binauthz_policy(project, evaluation_mode, enforcement_mode,
  global_evaluation)`; `gcp_binauthz_exempt_image(pattern)` per pattern
- `gcp_enabled_service(project, service)`

No type holds secrets; each schema says so in its comment. No graph edges
yet (pudl-q4v.21).

## Collector mechanics (models)

- `schema/models/gcp_hierarchy.cue`: `_gcpOrgLevel` names the four types
  above projects. Their CAI search runs per organization in any
  `_gcpAssetScope`, the transform keeps records outside projects and sets no
  project, and the fallback fans out over `_gcpOrgList` with no
  `empty_when` (a disabled-API error names gcloud's quota project, not the
  organization). Network firewall policies (project-level) are skipped.
- `schema/models/gcp_project_settings.cue`: `_gcpAssetByProject` makes the
  Binary Authorization search run per project (CAI names the project by
  number; the run's `set` names it by ID). Services shape: drop `config`,
  name = service DNS name; `cloudsecuritycompliance.googleapis.com` is
  skipped (CAI never exports it).
- `gcp_assets.cue`: `_gcpAssetTransform` tolerates a missing
  `parentFullResourceName` (organizations) and honours `_gcpOrgLevel`;
  `_gcpAssetSearch` honours both new sets.
- `gcp_pack.cue`: per-organization fan-out for org-level live collectors;
  fixtures set `project` only for project types, and the CAI fixture run of a
  per-project type sets it as the live search does.

## Verification

- Live census and end-to-end runs in a scratch workspace (nothing
  committed): the CAI collectors stored exactly what a census search
  returned. Re-run through gcloud: organizations, folders and the readable
  Binary Authorization policies stored no new records.
- Services through gcloud: a few new records, all
  `containeranalysis.googleapis.com` in projects where CAI omits it. No
  rule found; documented in the README and design findings.
- Org policy and firewall policy fallbacks could not be run by the surveying
  account (pudl-2ma).
- Tests: `cmd/gcp_pack_hierarchy_test.go` (fake gcloud: folders searched per
  organization in project mode and walked by the fallback with no new
  records; Binary Authorization searched per project, default policy
  dropped). The pack's type lists in `gcp_pack_test.go`,
  `gcp_pack_assets_test.go` (CAI vs gcloud fixtures identical) and
  `gcp_pack_inference_test.go` (org-level fixtures infer without a project)
  include the six types. `CGO_ENABLED=0 go test ./...` passes; doctor clean
  on the fixture workspace.

## Follow-ups

- pudl-q4v.21: graph edges for folders, firewall policy associations and
  org policy attachments.
- pudl-2ma: live parity for the org policy and firewall policy fallbacks.
