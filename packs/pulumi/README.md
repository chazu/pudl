# Pulumi content pack

An installable set of CUE files for cataloging Pulumi Cloud stacks with PUDL:
a draft schema, a live collector that reads Pulumi Cloud with GET requests
only, and an offline fixture collector. It contains no vendor-specific Go.

```bash
pudl init
pudl pack install pulumi
pudl check pulumi-fixture-stacks pulumi-fixture-resources   # fixtures; no Pulumi Cloud access
```

Installed files, under `.pudl/`:

| Path | Contents |
| --- | --- |
| `schema/pulumi/stack.cue` | `pulumi.#Stack`: a stack and its latest update (draft) |
| `schema/pulumi/resource.cue` | `pulumi.#Resource`: one resource of a stack's state (draft) |
| `schema/models/pulumi_pack.cue` | Settings, the transforms, the live and fixture collectors |
| `schema/models/pulumi_guard.cue` | The token guard and its read-scope allow list |
| `schema/pudl/rules/pulumi_pack.cue` | `pulumi_managed` and `pulumi_ancestor` |
| `populators/pulumi/*.json` | Fixtures: guard responses, a stack list in two pages, update histories, states |

## The live collectors

`pulumi-stacks` reads every stack of the organization and the latest update
of each. `pulumi-resources` reads the state of every stack (`GET
/api/stacks/<org>/<project>/<stack>/export`). Both use the `#HTTPObserve` populate source: GET requests only, to
`https://api.pulumi.com` only, with the token from `PUDL_PULUMI_TOKEN`.
pudl never uses the `pulumi` CLI, `PULUMI_ACCESS_TOKEN` or
`~/.pulumi/credentials.json`.

### 1. Make a read-only org access token

Pulumi state holds secrets, and some values in it are not marked secret. A
token with `stack:decrypt` or a write scope is not acceptable.

1. In Pulumi Cloud, create a custom role with read scopes only. It needs
   `stack:read` and `stack:export`. Do not use a role built on the built-in
   "Stack Read" permission set: that set includes `stack:decrypt`.
2. Create an **organization access token** with that role. Personal and team
   tokens are refused.
3. Export the token as `PUDL_PULUMI_TOKEN` in the shell that runs pudl.

### 2. Set the organization and the token name

Add them to a file of package `models`, for example `.pudl/schema/settings.cue`.
CUE adds a `package models` file in an ancestor directory of
`schema/models` to that package, so one file can hold every pack's settings:

```cue
package models

_pulumiOrg:       "my-org"
_pulumiTokenName: "pudl-reader"
```

Until both are set, the live collectors fail before it sends a request, with
`invalid interpolation: non-concrete value` at `observation.scope`.
`_pulumiAPI` changes the API origin, for a self-hosted Pulumi Cloud only.

### 3. Run it

```bash
pudl check pulumi-stacks pulumi-resources
```

### The token guard

Before any other request, pudl sends four GET requests: `/api/user`, and the
role, policy and permission-set lists of `/api/orgs/<org>/roles`. The
populate stops, and pudl sends nothing else, unless all of these are true:

1. The token is the org access token `_pulumiTokenName` of `_pulumiOrg`.
2. The token has exactly one role.
3. Every descriptor in the role's tree, and in the policies and sets that
   it names, has a known type. Every ID resolves, and the tree is at most
   four levels deep and three references long.
4. Every scope that the role reaches is in `_pulumiReadScopes`. That list
   holds Pulumi's read, list and view scopes and `stack:export`. It holds
   no decrypt, encrypt, open, write, create, update or delete scope.

The error names the condition that failed. If Pulumi adds a read scope
and your role includes it, the guard refuses the token until the scope is
added to `_pulumiReadScopes` in this pack.

## `pulumi.#Stack`

One record per stack that has at least one update. Identity: `stack`
(`<org>/<project>/<stack>`).

| Field | From the latest update |
| --- | --- |
| `version`, `kind`, `result`, `startTime`, `endTime`, `resourceCount` | The same fields |
| `pulumiVersion` | `environment["pulumi.version"]` |
| `source.repo` | `vcs.owner` + `/` + `vcs.repo` |
| `source.vcs`, `source.root` | `vcs.kind`, `vcs.root` |
| `source.commit`, `source.branch` | `git.head`, `git.headName` |
| `source.dirty` | `git.dirty == "true"`: the commit is approximate |

The transform keeps only these fields. It drops the update's `message`,
`config`, `deployment` and `resourceChanges`, and all other environment
keys, for example `git.author*`, `git.committer*`, `pulumi.env.*` and
`pulumi.flag.*`.

The observation scope is `pulumi/<org>/stacks`. A run for one organization
does not mark the stacks of another organization as gone. A stack that
fails to read is a failed input, and then nothing is marked gone.

## `pulumi.#Resource`

One record per resource in a stack's state, components included. Identity:
`urn`.

| Field | From the state resource |
| --- | --- |
| `urn`, `type`, `id`, `custom`, `external`, `protect`, `parent`, `dependencies`, `created`, `modified`, `sourcePosition` | The same fields (`external` is `false` when absent) |
| `provider` | The provider reference without its `::<id>` suffix |
| `live.metadata.uid`, `.name`, `.namespace` | `outputs.metadata` of `kubernetes:*` types, plain strings only |

Pulumi state holds secrets that Pulumi does not mark as secret: shell
commands and their environment, Secret and ConfigMap data, the
`last-applied-configuration` annotation, database passwords and provider
credentials. The transform keeps none of them. It drops `inputs`, every
output except the `live` values, `propertyDependencies`,
`additionalSecretOutputs` and `initErrors`. A secret envelope at a `live`
path is dropped, and `reject_keys` stops the populate if an envelope is
still in a record. An export with no `resources` array is a stack with no
records, not a failed input.

## Facts and rules

| Relation | Arguments |
| --- | --- |
| `pulumi_resource` (fact) | `urn`, `stack`, `type`, `custom`, `external`, `parent`, `has_parent` |
| `pulumi_stack_source` (fact) | `stack`, `repo`, `root`, `commit`, `dirty` |
| `pulumi_managed` (rule) | `urn`, `stack`: `custom` and not `external`, providers included |
| `pulumi_ancestor` (rule) | `urn`, `ancestor`: every resource above `urn` in its parent chain |

A root resource has no `parent` argument. Rules that join on `parent` match
`has_parent: true` first, because a missing argument reads as `null`.

```bash
pudl query pulumi_ancestor urn=<urn>
```

## Fixtures

The response shapes come from a live check with a read-only org token
(2026-11-10). All values are placeholders. Each `updates-<name>.json` file
is the `updates` array of one stack's `/updates` response, and each
`resources-<name>.json` file is the `deployment.resources` array of one
export. `resources-dev.json` is hostile: it holds a kubeconfig provider, a
Secret with a last-applied annotation, a ConfigMap with a secret envelope
as its name, a `command:local:Command` with env values and a SQL user with
a plaintext password. Planted values (`PLANTED-*`) must never reach the
catalog. `cmd/pulumi_pack_test.go` and `cmd/pulumi_pack_resources_test.go`
search every stored file for them.
