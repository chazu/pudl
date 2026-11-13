# GCP pack: deployment image and Argo instance label facts (2026-11-13)

Closes `pudl-avqx.2`. `gcp.#K8sDeployment` records kept each container's
image but projected none of it. The pack now projects the images, the
registry parts of their references, `runs_image` edges to
`gcp.#ContainerImage`, and the Argo CD instance labels.

## Facts (`packs/gcp/schema/gcp/k8s.cue`)

| Relation | `each` | Args |
|---|---|---|
| `gcp_k8s_deployment_container` | `spec.template.spec.containers[*]` | `container`, `image`, `registry`, `tag`, `digest` |
| `gcp_k8s_deployment_init_container` | `spec.template.spec.initContainers[*]` | same |
| `gcp_k8s_deployment_image` | `spec.template.spec.containers[*]` | `container`, `image` (the `gcp.#ContainerImage` name) |
| `gcp_k8s_deployment_init_image` | `spec.template.spec.initContainers[*]` | same |
| `gcp_k8s_deployment_argo_instance` | | `instance` from `metadata.labels."argocd.argoproj.io/instance"` |
| `gcp_k8s_deployment_app_instance` | | `instance` from `metadata.labels."app.kubernetes.io/instance"` |

Init containers have their own relations because a facts relation fans out
over one list. The reference parts come from rewrites on the image string:
`registry` is the first path segment when it holds a `.` or `:` (or is
`localhost`), else `docker.io`; `tag` and `digest` are `""` when the
reference carries none, so every container yields a fact. The
`gcp.#ContainerImage` name rewrite (tag and digest dropped, gcr.io hosts
mapped to the Artifact Registry repositories that serve them) moved from
`#RunService` to `_gcpArtifactImageRewrite` in `artifacts.cue`, shared by
Cloud Run and deployments. A container running an image from elsewhere
yields no `*_image` fact and no edge.

The instance-label facts use a rewrite that accepts any value, so a
deployment without the label yields no fact (Datalog has no negation).

## Rules

- `gcp_graph_k8s_deployment_image`, `gcp_graph_k8s_deployment_init_image`
  (`pudl/rules/gcp_graph.cue`): `runs_image` edges, deployment → image,
  resolved to the cataloged `gcp.#ContainerImage` or drawn as a reference.
- `gcp_k8s_deployment_argo_app(resource_id, application, via)`
  (`pudl/rules/gcp_pack.cue`): the union of the two label facts, `via`
  naming the label. An Argo CD pack can join on `application`.

## Fixture and tests

`populators/gcp/cai/k8s-deployments.json`: `checkout` gains the Argo label,
a `proxy` sidecar on a `gcr.io` image with tag and digest, and a `migrate`
init container on a digest-only image; `catalog` gains the recommended
label and a Docker Hub `cache` sidecar.

- `cmd/gcp_pack_k8s_test.go`: `TestGCPPackK8sDeploymentFixture` checks
  every fact above against the fixture.
- `cmd/graph_export_test.go`: the full `runs_image` edge set, Cloud Run and
  deployments, including the gcr.io image resolved to its cataloged
  Artifact Registry record.

`packs/gcp/README.md` documents the facts and the widened edge.
