# Pulumi pack: link cloud resources to the stacks and repo that manage them

Status: accepted 2026-11-06. Epic: `pudl-7g8j`. Builds on
[shared identity across schemas](2026-11-05-shared-identity-across-schemas.md)
and [GCP collectors on Cloud Asset Inventory](2026-11-03-gcp-cloud-asset-collectors.md).

## Problem

pudl sees live GCP and Kubernetes resources, but not the IaC that manages
them. This design covers Pulumi stacks that keep their state in Pulumi
Cloud, with their programs in a git repo. For each live resource, pudl must show the Pulumi stack that manages it, the
repo directory of that stack, and the commit of its last update.

Pulumi state holds credentials and secrets that Pulumi does not mark as
secret. The collection must keep only what it needs and must fail closed.

## Decisions (2026-11-06, with the user)

1. **Read-only token only.** pudl reads the token from `PUDL_PULUMI_TOKEN`
   and refuses a token that it cannot prove to be the configured org token
   with a read-only role (section 3). pudl never uses the `pulumi` CLI, `PULUMI_ACCESS_TOKEN` or
   `~/.pulumi/credentials.json`, which hold a personal token with write
   access.
2. **Collectors in CUE.** No shell helpers and no `jq`. A new core populate
   source, `#HTTPObserve`, sends GET requests. CUE declares the requests and
   the transforms.
3. **Not `tools/flow`.** Flow runs a DAG once and cannot follow a page token.
   The stock `tool/http` task accepts any method. Flow results do not feed
   pudl's observation report (failed inputs, completeness, gone-marking).
   Add flow later behind a GET-only task if a collector needs a chain that
   `each` → `runs` cannot express.
4. **Allow list, not deny list.** Each transform builds its output from named
   fields only. pudl drops `inputs` and every output that is not an identity
   field.
5. **Pulumi records draw no graph node of their own.** A Pulumi record shows
   as a representation of a live resource's group. Components (`custom:
   false`) stay in the data with their parent chain and show as a label.
   "Group by component" in the UI is a later option.
6. **Repo link from update history.** `vcs.repo` and `vcs.root` of the latest
   update give the repo and directory. pudl does not scan repos.

## Findings that shape the design (2026-11-06)

- **Backend.** With Pulumi Cloud and its default secrets provider, Pulumi
  Cloud holds the encryption key. A token without `stack:decrypt` gets
  ciphertext only.
- **Update history.** `environment` has `vcs.kind`, `vcs.owner`, `vcs.repo`,
  `vcs.root`, `git.head`, `git.headName`, `git.dirty`, `git.author`,
  `git.author.email`, `git.committer`, `git.committer.email`, `pulumi.os`,
  `pulumi.arch`, `pulumi.version`, `pulumi.flag.*` and `pulumi.env.*`. There
  is no `git.remote`. An update from a working tree with uncommitted changes
  records `git.dirty: "true"`.
- **Stacks outside the current tree.** A stack can outlive the directory of
  its project in the repo. `vcs.root` still names the directory that each
  update ran from.
- **Endpoints.** From the CLI's own client
  (`pkg/backend/httpstate/client/client.go` in `pulumi/pulumi`):

  | Request | Response |
  |---|---|
  | `GET /api/user` | `githubLogin`, `organizations`, `tokenInfo{name, organization?, team?}`. No `tokenInfo` for a personal token. |
  | `GET /api/user/stacks?organization=O[&continuationToken=T]` | `stacks[{orgName, projectName, stackName, lastUpdate, resourceCount}]`, `continuationToken?` |
  | `GET /api/stacks/O/P/S/export` | `{version, deployment{manifest, secrets_providers, resources[]}}` |
  | `GET /api/stacks/O/P/S/updates?pageSize=N&page=P` | `updates[{kind, startTime, endTime, version, result, message, environment, config, deployment?, resourceChanges, resourceCount}]` |

  Decrypt is `POST /api/stacks/O/P/S/decrypt`. A GET-only pudl cannot call it.
- **Sensitive data with no secret mark.** These types can hold it:
  `command:local:Command` (inputs hold shell commands and their env),
  `kubernetes:core/v1:Secret`, `kubernetes:core/v1:ConfigMap`,
  `gcp:sql/user:User`, and the provider resources (kubeconfig, credentials).
  The `kubectl.kubernetes.io/last-applied-configuration` annotation can hold
  a full Secret. An update's `config` and `deployment` hold stack config and
  a full state.

## Design

### 1. Core: `#HTTPObserve`

The new source is part of the populate disjunction in
`internal/systemmodel/schema.cue`:

```cue
#HTTPObserve: {
	schema?:                string
	allow_schema_fallback?: bool | *false
	// base is the only origin that pudl sends requests and the token to.
	base: =~"^https://[^/]+$"
	auth?: {
		env:    =~"^[A-Z][A-Z0-9_]*$" // the variable that holds the token
		header: string | *"Authorization"
		scheme: string | *"Bearer"
	}
	guard?:       #HTTPGuard
	each?:        #HTTPEnumeration
	runs:         [#HTTPRun, ...#HTTPRun]
	timeout?:     string
	fail_fast?:   bool | *false
	reject_keys?: [...string & !=""]
}

#HTTPPath: =~"^/" // relative to base: no scheme, no host

#HTTPGuard: {
	get: #HTTPPath
	check: {in: _, ok: bool, reason?: string}
}

#HTTPEnumeration: {
	get:  #HTTPPath
	list: string                  // field path of the array in the response
	item: {in: _, out: string}    // the input of one array element
	page?: {next: string, param: string} // response field -> query parameter
}

#HTTPRun: {
	get:        #HTTPPath
	records?:   string // field path of the record array; omit for one record
	set?:       {...}
	transform?: {in: _, out?: _}
}
```

Rules of the executor:

1. **GET only.** The schema has no method field. The Go client has one fixed
   method.
2. **Token.** pudl reads `auth.env` when the run starts. If the variable is
   not set or is empty, the populate fails. pudl sends the token only to the
   `base` origin and refuses a redirect to another origin. The token never
   goes into a report, an error or a log.
3. **Guard first.** Before `each` and `runs`, pudl sends `guard.get` and
   evaluates `check` with `in` bound to the response. If `ok` is false, or
   the evaluation fails (for example, a missing field), the populate fails
   with `reason`. pudl sends no other request.
4. **Each.** pudl pages through `list` with `page`. Each `item.out` must
   match `^[A-Za-z0-9._~/-]+$` and must not contain `..`. pudl replaces
   `{{item}}` in `get` and in the strings of `set`, as for commands.
5. **Memory only.** pudl reads each body under the `ingestprep.Limits` byte
   bound and decodes it with `UseNumber`. It selects `records`, applies
   `transform`, then `set`. The raw body never goes to disk. Records then
   follow the same ingest path as `#CommandObserve` records.
6. **`reject_keys`.** After the transform, if a record has one of these keys
   at any depth, the populate fails and pudl stores no record from it.
7. **Failures.** A non-2xx response is a failed input with its status code.
   pudl does not keep the response body.
8. **Report.** The result has the same shape as for commands: records,
   failed inputs and empty inputs. Observation completeness and gone-marking
   work with no change. `fallback`, `empty_when` and `fail_when` are not in
   the first version.

Code: `internal/populate/http.go` with tests on `httptest.NewTLSServer`,
the Go type and validation in `internal/systemmodel`, and dispatch in
`internal/operation/populate.go`. A new ratchet rejects outbound `net/http`
client calls outside `internal/populate/http.go`.

### 2. Pack layout and settings

`packs/pulumi/` has the same layout as the GCP pack: `bundle.go`,
`README.md`, `schema/pulumi/*.cue` (package `pulumi`),
`schema/models/pulumi_pack.cue`, `schema/pudl/rules/pulumi_rules.cue`, and
`populators/pulumi/` (JSON fixtures only). `internal/packs/packs.go`
registers it.

Settings are hidden fields with no defaults. The workspace must set them:

```cue
_pulumiOrg:       string & !="" // the Pulumi Cloud organization
_pulumiTokenName: string & !="" // the org access token pudl accepts
```

As built (`pudl-7g8j.3`), `_pulumiAPI` (default `https://api.pulumi.com`)
sets the API origin, for a self-hosted Pulumi Cloud and for tests.

### 3. Token guard

#### Spike findings (2026-11-06, `pudl-7g8j.2`)

- **A read-only role is possible.** Pulumi Cloud custom roles (Pro and
  Enterprise editions) are built from permission sets of scopes. The stack
  scopes are separate: `stack:read`, `stack:list`, `stack:export`,
  `stack:decrypt`, `stack:encrypt`, `stack:write`, `stack:import`,
  `stack:delete` and others. A set of `stack:read`, `stack:export` and
  other read scopes, with no `stack:decrypt` and no write scope, is what
  pudl needs.
- **The built-in "Stack Read" set is not safe.** It includes `stack:decrypt`
  and `stack:encrypt`. pudl must not use a token with a role built on it.
- **Org access tokens take exactly one role.** Team tokens get the union of
  their team's roles. Personal tokens get all of the user's permissions.
- **`/api/user` does not return a role.** `tokenInfo` has only `name`,
  `organization` and `team`.
- **The role can be read with GET requests.**
  `GET /api/orgs/O/roles?uxPurpose=role&assignedToMe=true` returns the
  caller's roles. A role's `details` is a tree of descriptors (`Compose`,
  `Group`, `Condition`) that refers to a policy by ID;
  `?uxPurpose=policy` lists policies and `?uxPurpose=set` lists permission
  sets with their `permissions`. Without `uxPurpose` the list returns HTTP
  400. `GET /api/orgs/O/roles/{id}` needs more rights than the list.

#### Live check (2026-11-10, `pudl-7g8j.10`)

An org token with a custom read-only role, GET requests only. Fixtures with
placeholder values: `packs/pulumi/populators/pulumi/guard-*.json`,
`stacks-page-*.json`, `updates-*.json` and `export.json`.

- **`/api/user`.** For an org token, `tokenInfo` is `{name, organization,
  team: null}`. The key `team` is present with the value `null`. The
  response also has user fields (`id`, `githubLogin`, `organizations`, ...).
- **Assigned role.** `?uxPurpose=role&assignedToMe=true` returns exactly one
  role. Its `details` can be a `PermissionDescriptorAllow` with the scopes
  inline. It then refers to no policy and no set.
- **Policies and sets.** The token lists both with no `role:read` scope.
  A policy is a `PermissionDescriptorGroup` of `entries`: a
  `PermissionDescriptorCompose` refers to sets by ID in
  `permissionDescriptors`, and a `PermissionDescriptorCondition` has a
  `condition` (for example `PermissionExpressionEqual` of
  `PermissionExpressionStack` and `PermissionLiteralExpressionStack`
  with a stack `identity`) and a `subNode`. A set has `resourceType`
  (`stack`, `environment`, `global`, ...) and a `PermissionDescriptorAllow`.
  The lists also hold the policies and sets of other roles.
- **Read scopes.** A read-only custom role holds read scopes of other areas
  too (environments, deployments, insights, policy packs, org metadata).
  Some reads do not end in `:read`, for example `environment:list_deleted`,
  `resources:search` and `resources:dashboard`. The guard must use an
  explicit list, not a name pattern. `environment:open` decrypts ESC values
  and must not be in the list.
- **Stacks.** `GET /api/user/stacks?organization=O` works with no
  `stack:list` scope. An item is `{id, orgName, projectName, stackName,
  lastUpdate, resourceCount, links{self}}`.
- **Export.** `stack:export` is sufficient. The secrets provider type is
  `service`. Each secret is an envelope with only the signature key and
  `ciphertext`, and no `plaintext` key is present. Resources can also have
  `additionalSecretOutputs`. A stack with no resources exports
  `deployment{manifest, metadata, secrets_providers}` with no `resources`.

#### Design

pudl accepts only an **org access token** with a **custom read-only role**.
The guard sends several GET requests and evaluates one check over all of
their responses (`pudl-7g8j.9` extends `guard.get` to a map of named paths):

```cue
_pulumiGuard: {
	get: {
		user:     "/api/user"
		roles:    "/api/orgs/\(_pulumiOrg)/roles?uxPurpose=role&assignedToMe=true"
		policies: "/api/orgs/\(_pulumiOrg)/roles?uxPurpose=policy"
		sets:     "/api/orgs/\(_pulumiOrg)/roles?uxPurpose=set"
	}
	check: {
		in: _
		ok: <see below>
		reason: "PUDL_PULUMI_TOKEN must be org token \(_pulumiTokenName) with a read-only role"
	}
}
```

`ok` is true only when all of these hold:

1. `in.user.tokenInfo.organization == _pulumiOrg` and
   `in.user.tokenInfo.name == _pulumiTokenName`, and `tokenInfo.team` is
   `null`. A personal or team token fails here.
2. The token has exactly one role.
3. Every descriptor that the role reaches has a known `__type`. The role's
   `details` is a `PermissionDescriptorAllow`, or refers to policies and
   sets by ID. Every scope that it reaches is in the explicit allow list:
   `stack:read`, `stack:list`, `stack:export`, `stack_access:read`,
   `stack_deployment:read`, `role:read`, and the other read scopes in
   Pulumi's scope reference (see "Read scopes" above). An unknown shape or
   an ID that is in no list fails the check.

A decrypt probe is not possible (it is a POST), and a write probe is never
acceptable.

As built (`pudl-7g8j.3`, `packs/pulumi/schema/models/pulumi_guard.cue`):
the walk is not recursive, because CUE 0.16 reports a cycle in a recursive
walk. Each tree is read four levels deep, and IDs are followed for three
rounds (role, policy, set). A deeper tree, or an ID cycle, fails the check.
`reason` names the condition that failed and, for condition 3, the scopes
outside the allow list.

### 4. `pulumi.#Stack`: one record per stack

Collector `pulumi-stacks`:

- `each`: `GET /api/user/stacks?organization=\(_pulumiOrg)`, `list: "stacks"`,
  `item.out: "\(in.orgName)/\(in.projectName)/\(in.stackName)"`, `page:
  {next: "continuationToken", param: "continuationToken"}`.
- `runs`: `GET /api/stacks/{{item}}/updates?pageSize=1&page=1`, `records:
  "updates"`, `set: stack: "{{item}}"`.

The transform keeps `stack`, `version`, `kind`, `result`, `startTime`,
`endTime`, `resourceCount`, `pulumiVersion` (from `pulumi.version`), and:

```
source: {repo: "<vcs.owner>/<vcs.repo>", vcs: vcs.kind, root: vcs.root,
         commit: git.head, branch: git.headName, dirty: git.dirty == "true"}
```

It drops `message`, `config`, `deployment`, `resourceChanges`, and all other
`environment` keys (`git.author*`, `git.committer*`, `pulumi.env.*`,
`pulumi.flag.*`, `pulumi.os`, `pulumi.arch`). Identity: `stack`. A new update
gives a new version of the record. A stack with no updates gives no record.

As built (`pudl-7g8j.3`): the observation scope is `pulumi/<org>/stacks`,
so a run for one organization marks nothing of another one gone. The live
check of 2026-11-10 confirmed the update shape: the newest update comes
first, and `resourceCount` can be absent.

### 5. `pulumi.#Resource`: one record per state resource

Collector `pulumi-resources`: the same `each`, and `runs`: `GET
/api/stacks/{{item}}/export`, `records: "deployment.resources"`, `set:
stack: "{{item}}"`.

The transform keeps:

- `urn`, `type`, `id`, `custom`, `external` (default `false`), `protect`,
  `parent`, `dependencies`, and `created`, `modified` and `sourcePosition`
  when present.
- `provider`: the URN part of the provider reference only (the text before
  the last `::`).
- `live`: the identity values of the real resource, built from `id` and from
  outputs named in a per-type table (`_pulumiIdentity`). Every `kubernetes:*`
  type keeps `metadata.uid`, `metadata.name` and `metadata.namespace`. For
  GCP types, `live.name` is the relative resource name (section 6). pudl keeps
  only string values. A secret envelope at an allowed path is dropped.

It drops `inputs`, all other `outputs`, `propertyDependencies` and
`initErrors`. `reject_keys: ["4dabf18193072939515e22adb298388d",
"ciphertext"]`. Identity: `urn` (it contains the stack and project).

As built (`pudl-7g8j.4`): `live` holds only the `kubernetes:*` metadata
values. The per-type GCP table that builds `live.name` is part of
`pudl-7g8j.5`. A stack with no resources exports no `resources` array, so
the run sets `records_optional: true`, a new `#HTTPRun` field: a missing
records path is no records instead of a failed input. The transform also
drops `additionalSecretOutputs`.

### 6. Links to live records

`pulumi.#Resource` declares `_pudl.same_resource` keys (shared identity
design):

- **`k8s.object`**, key `uid: "live.metadata.uid"`. This links to
  `gcp.#K8sDeployment` today and to the Kubernetes pack (`pudl-3q5`) later.
- **`gcp.resource`** (new kind), key `name`: the relative resource name, for
  example `projects/p/global/networks/n`. On the Pulumi side, a per-type
  table builds `live.name` from `id`. On the GCP side, each schema in the
  first set declares the key from `selfLink` with `trim_prefix`, or from
  `name` with `prepend`. No record changes.

First GCP set (common types that the GCP pack collects):
project, network, subnetwork, address, forwarding rule, router, DNS managed
zone, DNS record set, GKE cluster, bucket, service account and Artifact
Registry repository (if collected). Verify each Pulumi `id` format against a
live export during delivery.

IAM member resources (`gcp:projects/iAMMember:IAMMember` and the like) are
bindings inside the IAM policy records, not resources. They get no link in
the first version.

As built (`pudl-7g8j.5`): the first set is network, subnetwork, address
(regional and global), forwarding rule (regional and global), router, GKE
cluster, bucket and service account. Projects and Artifact Registry
repositories are not collected by the GCP pack, so they wait. DNS managed
zones and record sets wait too: their GCP records hold the project and the
zone as separate fields, so no `trim_prefix`/`prepend` key forms the
relative name, and a record change or a multi-part key is a decision for
later. Two quirks: a zonal GKE cluster's `selfLink` says `zones/Z` where
Pulumi says `locations/Z`, so the Pulumi side rewrites zone-shaped
locations; a bucket's Pulumi `id` is the bare name, so both sides use
`b/<name>`. The `id` formats were verified against live exports (open
question 2). Both packs declare the
`gcp.resource` kind with the same rule, so either passes doctor alone.

### 7. Facts and rules

Projected facts:

- `pulumi_resource(urn, stack, type, custom, external, parent)`
- `pulumi_stack_source(stack, repo, root, commit, dirty)`

Rules:

- `pulumi_managed(urn, stack)` from `pulumi_resource` with `custom: true`
  and `external: false`.
- `pulumi_ancestor(urn, ancestor)`, recursive over `parent`, for the
  component path label.

As built (`pudl-7g8j.4`): `pulumi_resource` also has `has_parent` (`{exists:
"parent"}`). A missing argument reads as `null` in a rule join, so the
`pulumi_ancestor` rules match `has_parent: true`. `pulumi_managed` includes
provider resources, which are `custom` too. The rules are in
`schema/pudl/rules/pulumi_pack.cue`, named like the other packs' rules.

### 8. UI

- A schema flag (`_pudl.graph`, for example `member_only: true`) makes
  pudl ui draw a record only as a representation of a group with another
  member. A Pulumi record with no live partner draws no node. It stays in
  `pudl list`, `pudl show` and `pudl query`.
- The inspector of a node with a `pulumi.#Resource` member shows the stack,
  `repo` and `root`, the commit (marked "approximate" when `dirty`), and the
  component path.

### 9. Fixtures and tests

- `populators/pulumi/`: fixtures for `/api/user` (team, org and personal
  tokens), a stack list in two pages, updates and exports.
- Fixture collectors (`pulumi-fixture-*`) use `#CommandObserve` with `cat`
  and the same CUE transforms. `#CommandObserve` has no `records` path, so
  an update fixture is the `updates` array of the response. Go tests on `httptest.NewTLSServer` cover
  the HTTP behaviour.
- **Hostile fixture.** An export with a kubeconfig provider, a Secret with a
  last-applied annotation, a `command:local:Command` with env values, a
  `gcp:sql/user:User` with a plaintext password, and a secret envelope at an
  allowed path. Also an update with `config`, `deployment` and
  `pulumi.env.*`. The test ingests them and searches every stored byte
  (payloads, facts, catalog) for each planted value. It must find zero.
- Guard tests: a personal token, a team token, another org token, a role with
  `stack:decrypt` or a write scope, and an unknown descriptor shape all
  fail, and the server sees no request after the guard.

## Delivery

| Issue | Work | Size |
|---|---|---|
| `pudl-7g8j.1` | Core `#HTTPObserve`: schema, executor, guard, paging, `reject_keys`, ratchet, tests | about 1 day |
| `pudl-7g8j.2` | Spike: whether a read-only role exists and how a token can read it (done) | about 1 hour |
| `pudl-7g8j.9` | Core: `guard.get` as a map of named requests, one check over all | about 2 hours |
| `pudl-7g8j.10` | Live check with an org token that has a read-only role (done) | about 30 minutes |
| `pudl-7g8j.3` | Pack skeleton, settings, guard, `pulumi.#Stack`, fixtures, README (done) | about half a day |
| `pudl-7g8j.4` | `pulumi.#Resource`, allow list, hostile fixture test, facts and rules (done) | about 1 day |
| `pudl-7g8j.5` | `k8s.object` key, `gcp.resource` kind and the first GCP set | about half a day |
| `pudl-7g8j.6` | `member_only` graph flag and the inspector section | about half a day |
| `pudl-7g8j.7` | Check: live GCP resources that no Pulumi stack manages | about 2 hours |
| `pudl-7g8j.8` | UI: group by Pulumi component | about 1 hour |

## Open questions

1. Answered (`pudl-7g8j.10`): `assignedToMe` returns the org token's one
   role, and listing policies and sets does not need `role:read`.
2. Answered (2026-11-12, live exports of every stack, in memory): every
   first-set type's `id` is the relative resource name (`projects/P/...`),
   except buckets (the bare name). No stack held a zonal GKE cluster, so
   the `zones/Z` rewrite rests on the `selfLink` format alone. Also seen:
   projects `projects/P`, Artifact Registry repositories
   `projects/P/locations/L/repositories/R`, DNS zones
   `projects/P/managedZones/Z`, record sets
   `projects/P/managedZones/Z/rrsets/NAME/TYPE`.
3. Answered (`pudl-7g8j.4`): live state has `sourcePosition`, and pudl keeps
   it.

## Out of scope

- Moving the GCP shell helpers to `#HTTPObserve`.
- Links for IAM member resources.
- The Pulumi Cloud resource search API.
- Where collection runs (a workstation or CI). The token moves with it, and
  pudl does not change.
