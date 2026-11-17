# Argo CD content pack

An installable set of CUE files for cataloging Argo CD Applications and
ApplicationSets with PUDL through `kubectl`, and for linking them to the
clusters and workloads they deploy. It contains no vendor-specific Go. The
design is `docs/design/2026-11-13-argocd-pack.md` in the PUDL source.

The pack needs the Kubernetes pack: the live collectors use its cluster
helper and kubectl run shape. The GCP pack is optional; with it, Argo CD
destinations and managed Deployments link to GKE clusters and to
`gcp.#K8sDeployment` records.

```bash
pudl init
pudl pack install k8s argocd gcp
pudl check argocd-fixture-applications argocd-fixture-applicationsets argocd-fixture-repositories   # fixtures; no cluster access
pudl check argocd-applications argocd-applicationsets argocd-repositories                           # the cluster of kubectl's current context
```

Installed files, under `.pudl/`:

| Path | Contents |
| --- | --- |
| `schema/argocd/application.cue` | `argocd.#Application` (draft) |
| `schema/argocd/applicationset.cue` | `argocd.#ApplicationSet` (draft) |
| `schema/argocd/repository.cue` | `argocd.#Repository` (draft) |
| `schema/argocd/redaction.cue` | The Helm redaction paths and `_argocdImageParameters` |
| `schema/models/argocd_pack.cue` | The collectors |
| `schema/models/argocd_shape.cue` | The allow-list transforms |
| `schema/models/argocd_repository.cue` | The repository URL normalization and the repository records |
| `schema/models/argocd_settings.cue` | `_argocdNamespaces` |
| `schema/pudl/rules/argocd_pack.cue` | Target host, cluster and workload joins, inspector rows |
| `schema/pudl/rules/argocd_graph.cue` | Edges and edge types |
| `populators/argocd/*.json` | Fixtures: `kubectl get -o json` Lists, and hostile Lists |

## The live collectors

Run Argo CD's collectors against the cluster Argo CD runs in. Each reads
kubectl's **current context** only: `kubectl get applications.argoproj.io
-A -o json` and `kubectl get applicationsets.argoproj.io -A -o json`, plus
the Kubernetes pack's read-only cluster helper. They run no other command.
Every record's `cluster` is the current context's API server host, which
is the Kubernetes pack's cluster record. As in the Kubernetes pack, the
collectors declare `complete: false`: a context switch marks nothing gone.

`argocd-repositories` reads the same two lists and writes one
`argocd.#Repository` record per repository their sources and git
generators name. A repository record has no `cluster`: one repository is
one record whatever cluster names it.

## Settings

| Setting | File and package | Default | Meaning |
| --- | --- | --- | --- |
| `_argocdNamespaces` | any file of package `models`, e.g. `schema/settings.cue` | `["argocd"]` | The namespaces whose objects are kept. The collectors list every namespace; `[]` keeps all of them (Applications in any namespace) |
| `_argocdImageParameters` | a file of package `argocd`, e.g. `schema/argocd/settings.cue` | `["image.name", "image.tag", "image.repository"]` | Helm parameters whose values are kept |

## Records and redaction

The transforms keep an allow list and drop everything else. An
Application record has `metadata` (no `managedFields`), `project`,
`sources`, `destination`, `destinationHost`, `syncPolicy` and the status
fields facts need. `spec.source` and `spec.sources` become one `sources`
list. Each source has its `index` and the revision it was last synced to
(`syncRevision`). These are dropped:

- plugin `env` and `parameters`
- `directory.jsonnet`
- kustomize patches
- `ignoreDifferences` and `info`
- the history entries' sources
- the operation's sources and its sync result
- every annotation whose key does not start with `argocd.argoproj.io/` or
  `argocd-image-updater.argoproj.io/`: `kubectl.kubernetes.io/last-applied-configuration`,
  notification subscriptions and a team's own annotations included

An ApplicationSet keeps its template, through the same allow list, and a
summary of each generator. The summary holds the type, a git generator's
repository, revision and path patterns, a list generator's element count,
a clusters generator's selector, and the children of a matrix or merge
generator, one level deep. List elements, generator template overrides
and `templatePatch` are dropped.

Every `repoURL` the transforms keep (a source's, in an Application and in
an ApplicationSet template, and a git generator's at the top level and in
a matrix or merge) loses the credentials it can carry as userinfo, so the
facts keep a usable URL:

| Stored `repoURL` | From |
| --- | --- |
| `https://git.example.com/org/repo.git` | `https://user:TOKEN@git.example.com/org/repo.git` or `https://TOKEN@git.example.com/org/repo.git`; the same for any scheme other than `ssh` |
| `ssh://git@git.example.com/org/repo.git` | `ssh://git:PASSWORD@git.example.com/org/repo.git`: an `ssh` URL keeps the user name and drops a password |
| `git@git.example.com:org/repo.git` | the same: the scp form has a user name only and is kept |

A URL without userinfo is kept as it is. Status keeps no `repoURL`: the
history entries' and the operation's sources are dropped.

Next to each `repoURL`, `repo` is the repository it names, normalized: no
userinfo; the `ssh`, `git`, `git+ssh` schemes and the scp form become
`https` without a port; the host is lower-cased and loses a default port;
the path keeps its case and loses the query, fragment, trailing slash and
`.git`. `git@git.example.com:org/repo.git`,
`ssh://git@Git.Example.com/org/repo.git` and
`https://git.example.com/org/repo/` are all
`https://git.example.com/org/repo`. A `repoURL` that holds a template
(`{{.repoURL}}`) has no `repo`.

## Repositories

`argocd.#Repository` is one repository: `url` (the normalized URL, its
identity), `name` (the URL without its scheme, drawn on the node) and
`type`: `helm` for a source that names a chart, `oci` for an `oci://` URL
without a chart, `git` otherwise and for every git generator. A git
repository also has `gitURL`, its `git.repo` key (a built-in identity
kind): another pack's record of the same git repository, keyed by the same
normalized URL, is the same graph node.

The annotations kept, on an Application, an ApplicationSet and an
ApplicationSet template, lose userinfo the same way: every value that is a
URL with a scheme, such as Image Updater's
`argocd-image-updater.argoproj.io/git-repository`, is stored as the table
above shows. Labels are kept as they are.

Then `sensitive_fields` redacts these values before anything is stored:

- Helm `values` and `valuesObject`
- file parameter paths
- every parameter value whose name is not on `_argocdImageParameters`

The last-applied annotation is not redacted: the annotation allow list
drops it before redaction runs, so `sensitive_fields` has no entry for it.

A parameter's name stays, so an override is visible. The ApplicationSet
template's Helm paths are redacted the same way. The hostile fixtures plant
values at each of these places, repository credentials in `repoURL` and in
an Image Updater annotation, and values in annotations off the allow list;
the pack's tests find none of them stored.

## Facts

| Relation | Arguments |
| --- | --- |
| `argocd_app` | `cluster`, `namespace`, `name`, `project`, `sync_status`, `health_status`, `sync_revision`, `reconciled_at`, `automated`, `self_heal`, `prune` |
| `argocd_app_source` | One per source: `index`, `repo_url`, `repo`, `path`, `chart`, `target_revision`, `ref`, `sync_revision` |
| `argocd_app_destination` | `server`, `name`, `namespace` (`""` when unset) |
| `argocd_app_destination_host` | `host`: the server URL's host, unless the server is the in-cluster one |
| `argocd_app_resource` | One per managed resource: `group`, `version`, `kind`, `namespace`, `name`, `status`, `health` |
| `argocd_app_image` | `image`: one per image Argo CD rendered |
| `argocd_app_history` | One per sync: `id`, `revision`, `deployed_at`, `automated` |
| `argocd_app_kustomize_image` | `index`, `image`: kustomize image overrides (Image Updater's `argocd` write-back) |
| `argocd_app_helm_parameter` | `index`, `name`, `value` (`[REDACTED]` unless allow-listed) |
| `argocd_app_image_updater` | `image_list`, `write_back_method` (default `argocd`), `write_back_target` |
| `argocd_app_owner` | One per owner reference: `kind`, `name`, `uid` |
| `argocd_appset` | `cluster`, `namespace`, `name`, `uid`, `generator_types` (e.g. `matrix(clusters,list)`) |
| `argocd_appset_source` | One per template source: `index`, `repo_url`, `repo`, `path`, `chart`, `target_revision`, `ref` |
| `argocd_appset_git_generator`, `argocd_appset_child_git_generator` | `repo_url`, `repo`, `revision`: git generators at the top level and inside a matrix or merge |
| `argocd_repo` | `url`, `name`, `type`: one per repository record |

Rules (`pudl query <relation>`):

| Relation | Meaning |
| --- | --- |
| `argocd_app_target_host(resource_id, host)` | The host of the cluster an Application deploys to. The in-cluster server is the record's own `cluster`. A destination given by name has none: resolving it needs Argo CD's cluster Secrets |
| `argocd_cluster(resource_id, host)` | A cluster record by host: the Kubernetes pack's cluster record, or a GKE cluster's public or DNS endpoint (`gcp_gke_cluster_endpoint`) |
| `argocd_workload_app(workload, application)` | A `gcp.#K8sDeployment` that an Application lists among its managed resources (group `apps`, kind, namespace, name), in the cluster the Application deploys to |
| `argocd_workload_app_confirmed(workload, application, via)` | The join above, confirmed by the workload's instance label or `argocd.argoproj.io/tracking-id` annotation naming the Application |
| `argocd_app_last_deploy(resource_id, deployed_at)` | The latest `deployedAt` in the history |

A label can be set by hand, so the label and the tracking-id annotation
check the `deployed_by` join and are not its basis. The tracking-id
annotation names an Application outside Argo CD's own namespace as
`<namespace>_<name>`; such an Application does not confirm.

## Graph

| Edge | From | To |
| --- | --- | --- |
| `in_cluster` | Application, ApplicationSet | The cluster Argo CD runs in |
| `generated_by` | Application | The ApplicationSet whose UID its owner reference names |
| `deploys_to` | Application | The cluster its destination names, by server URL or in-cluster |
| `deployed_by` | `gcp.#K8sDeployment` | The Application that manages it |
| `sourced_from` | Application, ApplicationSet | Each repository its sources name, and for an ApplicationSet its template sources and git generators (also inside a matrix or merge). Properties: `path`, `chart`, `target_revision`, and a git generator's `revision` |

`sourced_from` joins a source's `repo` with the `url` of a repository
record. A repository no record backs has no edge and no node: run
`argocd-repositories` (or its fixture twin) to draw them.

Display codes: `APP` (Argo CD application), `APSET` (Argo CD
application set) and `REPO` (repository). pudl ui allows codes of 5 characters or fewer. The
inspector's "Argo CD" section is on each Application and on each workload
it deploys. It shows the application, sync status, health, last deploy and
Image Updater write-back method. It also shows the repo, path, chart,
target revision and synced revision of each source, ranked by the source's
index.

## Fixtures

The fixtures were collected from the GCP fixture cluster `batch-private`
(its DNS endpoint). Argo CD runs there and deploys to `prod-private` by
its server URL, as a central Argo CD does. The fixture Applications are:

- `cluster-addons`: a Helm chart with the in-cluster destination
- `shop-checkout`: multi-source, `ref: values` plus a chart
- `shop-catalog`: generated by the ApplicationSet `shop`
- `edge-proxy`: kustomize with Image Updater annotations and a written-back
  image, with a named destination, and the manifests repository in the scp
  form
- `sandbox-demo`: in a namespace the default `_argocdNamespaces` drops

The ApplicationSets are `shop` (git directories), `platform` (matrix of
clusters and list) and `regional` (merge of git files and clusters, with
its template's repository in the ssh form). The
`*-hostile.json` Lists plant fake secrets. All values are placeholders.
