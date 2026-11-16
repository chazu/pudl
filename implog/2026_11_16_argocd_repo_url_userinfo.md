# Argo CD pack: redact credentials in repoURL (2026-11-16)

A `repoURL` can carry repository credentials as userinfo
(`https://user:TOKEN@git.example.com/org/repo.git`,
`https://TOKEN@git.example.com/...`). The Argo CD pack kept such values as
they were, in the records, the facts and the inspector rows. This is
`pudl-nap8`.

## The fix (`packs/argocd/schema/models/argocd_shape.cue`)

`_argocdRepoURL: {_url: string, out: string}` removes the userinfo of a
URL with a scheme. The pattern `_argocdRepoURLPattern` takes the userinfo
up to the last `@` of the authority (before the first `/`, `?` or `#`), so
an `@` in the path or query stays.

| Input | `out` |
| --- | --- |
| `https://user:TOKEN@host/path`, `https://TOKEN@host/path` (any scheme but `ssh`) | `https://host/path` |
| `ssh://git:PASSWORD@host/path` | `ssh://git@host/path`: the user name stays, the password goes |
| `ssh://git@host/path` | unchanged |
| `git@host:org/repo.git` (scp form, no scheme) | unchanged: a user name only |
| a URL without userinfo, or without a scheme | unchanged |

It is applied in the transform, not with `sensitive_fields`, so the facts
(`argocd_app_source` `repo_url`, `argocd_appset_git_generator` and
`argocd_appset_child_git_generator` `repo_url`) and the inspector's repo
row keep a usable URL. The places that keep a `repoURL`:

- `_argocdSource`: each source of an Application and of an ApplicationSet
  template (`spec.source` and `spec.sources`)
- `_argocdGeneratorLeaf`: a git generator's summary, at the top level and
  as a child of a matrix or merge (the only depths the summary keeps)

Status keeps no `repoURL`: `status.sync` keeps `status`, `revision` and
`revisions`; history entries and the operation drop their sources.

## Tests

The hostile fixtures plant `FAKE-ARGO-REPO-USER` and
`FAKE-ARGO-REPO-TOKEN` in https userinfo: in a multi-source source of
`hostile-app`, in the single source of a new Application `hostile-single`
(token as the user; a named destination, so no deploy edge), in the
`hostile-set` template, and in a git generator inside a new matrix
generator of `hostile-set`. Both are on `argocdPlanted`, so
`TestArgoCDPackHostileFixturesPlantNothing` proves no byte of them is
stored; it also checks the stored `repoURL`s, the `argocd_app_source` and
`argocd_appset_child_git_generator` facts. The Application count, the
hostile set's generator types and the child git generator count in the
fixture tests follow the new fixtures.

Checks: `make test`.
