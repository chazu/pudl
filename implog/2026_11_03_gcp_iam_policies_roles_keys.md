# GCP pack: IAM policies on every resource, custom roles, service account keys (pudl-q4v.5)

**Date:** 2026-11-03

## Work done

### Product: partial fallback

`populate.fallback.partial: true` declares a fallback that sees only some of
the populate's resources. When such a fallback answers, the observation is
incomplete, so resources it cannot see are not marked gone. The report adds
`- partial fallback: ...` (JSON `populate.fallback.partial`).

- `internal/systemmodel/schema.cue`, `fallback.go`: `partial?: bool`,
  `Fallback.Partial`.
- `internal/populate`: `FallbackUse.Partial`, set from the populate.
- `internal/operation/populate.go`: a partial fallback ingests with
  `Complete: false`.
- `internal/checkreport/populate.go`: the report line.
- `docs/observation-scope.md`: documented.

### GCP pack

| Collector | Schema | Identity | CAI source | gcloud source |
|---|---|---|---|---|
| `gcp-iam-policies` | `gcp.#IAMPolicy` (replaces `gcp.#ProjectIAMPolicy`) | `resource` | `search-all-iam-policies --scope=projects/P`, always per project | `projects get-iam-policy P` (project policy only; partial) |
| `gcp-iam-roles` (new) | `gcp.#IAMRole` | `name` | `iam.googleapis.com/Role` | `iam-roles.sh`: list, then describe each |
| `gcp-service-account-keys` (new) | `gcp.#ServiceAccountKey` | `name` | `iam.googleapis.com/ServiceAccountKey` | `service-account-keys.sh`: keys list `--managed-by=user` per account |

- IAM policy shape: `{resource, assetType, project, policy: {bindings}}`.
  The search's `folders`, `organization` and numeric `project` are dropped
  (the run sets the project ID); gcloud's `etag`, `version` and
  `auditConfigs` are dropped, and the run sets `resource` and `assetType`.
- A project that refuses the IAM search is a failed input. Only
  `SERVICE_DISABLED` and `USER_PROJECT_DENIED` (`_gcpIAMSearchUnavailable`)
  fall back, partially. With `_gcpAssetScope: "off"` the IAM observation is
  incomplete for the same reason (`_gcpPartialGcloud`).
- Role shape: drop `etag`, `groupName`, `groupTitle`; `stage` defaults to
  `ALPHA`. Organization roles are dropped by the CAI transform, which now
  drops any resource whose parent is not a project.
- Keys: Google-managed keys are dropped from both sources (`_gcpSkip`).
  `privateKeyData` and `publicKeyData` are sensitive, though neither source
  prints them.
- Facts: `gcp_iam_policy(asset_type)`, `gcp_iam_member(role, member,
  conditional)` per binding member (was `gcp_iam_binding`),
  `gcp_iam_role(name, stage)`, `gcp_iam_role_permission(name, permission)`,
  `gcp_service_account_key(name, key_type, valid_after, valid_before,
  disabled)`. The `iam_binding` graph edges now run from a member to the
  policy of any resource.
- Fixtures (synthetic): the project policy gains a custom-role binding and
  audit configs; CAI fixtures add bucket, service account and topic policies,
  an organization role and a Google-managed key, which are dropped.

### Live checks (field paths and counts only; no values kept)

- IAM search in one project: one result per resource, several asset types.
  Its project policy equals `get-iam-policy`'s bindings, order included.
- Some projects allow the search; the others refuse both the search
  and `get-iam-policy`.
- Roles: every project role matches after the shape. A stage-less CAI role
  is `ALPHA` in gcloud.
- Keys: the user-managed keys are identical. Google-managed keys barely
  overlap (only user-managed keys are shared).

## Public API

- `populate.fallback.partial` (CUE), `systemmodel.Fallback.Partial`,
  `populate.FallbackUse.Partial` (JSON `partial`).
- GCP pack: `gcp.#IAMPolicy`, `gcp.#IAMRole`, `gcp.#ServiceAccountKey`;
  `gcp.#ProjectIAMPolicy` removed. Models `#GCPIAMRoles`,
  `#GCPServiceAccountKeys` and fixture variants. Settings unchanged.
- Internal pack values: `_gcpIAMPolicySearch`, `_gcpIAMPolicyTransform`,
  `_gcpProjectPolicyRun`, `_gcpIAMSearchUnavailable`, `_gcpPartialGcloud`,
  `_gcpSkip`, `_gcpRoleShape`, `_gcpFixtureGcloudRun`.

## Tests

- `internal/operation/fallback_partial_test.go`: a partial fallback's
  snapshot is incomplete and the report says so; a plain one stays complete.
- `cmd/gcp_pack_iam_test.go` (fake gcloud): the search runs per project;
  a refused project is a failed input with no fallback; an unbillable
  search falls back partially, stores nothing new and leaves the snapshot
  incomplete; `off` does the same; the role and key helper scripts.
- `TestGCPPackAssetSourcesStoreIdenticalRecords` covers roles and keys.
- `TestGraphExportGCPPack`: bindings to project, bucket and topic policies.
- `CGO_ENABLED=0 go test ./...` passes. `pudl doctor` on a fresh fixture
  workspace reports only the known `.examples` warning (`pudl-bpy`).

## Upgrading a workspace

`pudl example install gcp --overwrite`. Stored `gcp.#ProjectIAMPolicy`
entries stay in the catalog under a schema the pack no longer defines. Rehearsed:
after the upgrade, `gcp-fixture-iam-policies` stores the new policies and
the one old entry remains. Remove old entries with `pudl list --schema
'gcp.#ProjectIAMPolicy'` and `pudl delete <id>`.

## Issues filed

- `pudl-42w`: a changed model's instance is recorded twice as version 1
  (seen during the upgrade rehearsal; reproduces on the previous commit).
- `pudl-q4v.16`: edge from an IAM policy to its resource.
- `pudl-q4v.17`: edge from a key to its service account (and key-age checks).
- `pudl-q4v.18`: organization-level custom roles.
