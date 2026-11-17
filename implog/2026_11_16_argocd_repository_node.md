# Argo CD pack: repository records and sourced_from (2026-11-16)

Builds `pudl-avqx.8`: the repository node for the graph, decided in
`docs/design/2026-11-13-argocd-pack.md` (decision 7 and section 7, "The
repository node"), then the `sourced_from` edge. `built_from` in
`pudl-avqx.5` is now possible; it is not built here.

## Decision

A repository is a record (`argocd.#Repository`), linked across packs by a
new built-in identity kind `git.repo`, keyed by the normalized repository
URL. Rejected: a core relation that lets rules declare a node
(`graph_node(id, type, name)`). A rule-declared node has no record, so no
evidence, history or `pudl show`, and graph export, the subgraph cut, time
travel, the vocabulary and `pudl doctor` would each need a path for it.
The choice adds one ground fact to the core and no Go code.

## Public surface

Core (`internal/importer/bootstrap/pudl/rules/same_resource.cue`):

- `same_resource_kind` fact for `git.repo` ("Git repository").

Argo CD pack:

| Item | What |
| --- | --- |
| `argocd.#Repository` (`schema/argocd/repository.cue`) | `url` (identity, normalized), `name` (URL without scheme), `type` (`git`, `helm`, `oci`), `gitURL` (git only; the `git.repo` key). Display `REPO`, class `data` |
| `argocd_repo(resource_id, url, name, type)` | Fact of each repository record |
| `repo` field | On each source (Application and template) and each git generator: the normalized `repoURL`; absent when `repoURL` holds a template |
| `argocd_app_source.repo`, `argocd_appset_git_generator.repo`, `argocd_appset_child_git_generator.repo` | New arg |
| `argocd_appset_source(resource_id, index, repo_url, repo, path, chart, target_revision, ref)` | New fact: an ApplicationSet's template sources |
| `argocd-repositories`, `argocd-fixture-repositories` | Collectors: one repository record per repository the Applications and ApplicationSets name (two runs, or four with the hostile fixtures) |
| `_argocdRepoID` (`schema/models/argocd_repository.cue`) | The normalization: `_argocdRepoURL` first, then ssh schemes and the scp form to https without a port, lower-case host without a default port, no query, fragment, trailing slash or `.git` |
| `sourced_from` edges | Application to each repository its sources name; ApplicationSet to each repository its template sources and git generators name (also inside a matrix or merge). Properties `path`, `chart`, `target_revision`, `revision` |

The edge rules join `repo` with `argocd_repo.url`, so a repository no record
backs has neither an edge nor a node.

## For pudl-avqx.5

Write a repository record from the image's
`org.opencontainers.image.source` label with
`same_resource: [{kind: "git.repo", key: url: <normalized URL>}]`, using
the same normalization (copy `_argocdRepoID`'s rules, or lift it into a
shared helper). The record and the Argo CD pack's record of the same
repository are then one node; `built_from` joins the image to the
pack's own record.

## Fixtures and tests

- `edge-proxy` names the manifests repository in the scp form and the
  `regional` ApplicationSet's template in the ssh form with an upper-case
  host; both reach the one repository record.
- `packs/argocd/repo_id_test.go`: `_argocdRepoID` on the embedded files;
  https, ssh, scp, `git+ssh`, port and userinfo forms give one URL, and the
  parts that name another repository are kept.
- `cmd/argocd_pack_repo_test.go`: the repository records (two, one
  `git.repo` key); `sourced_from` for single-source, multi-source,
  ApplicationSet template sources and git generators inside a matrix and a
  merge, with properties; no edge, node or unresolved reference without
  the repository records.
- The hostile test runs the repository collector too and still finds no
  planted byte.
- `internal/projection/same_resource_test.go`: three built-in kinds.
