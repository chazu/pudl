# Argo CD pack: design doc (pudl-avqx.3)

Date: 2026-11-13

Wrote `docs/design/2026-11-13-argocd-pack.md`, the design for a content
pack that collects `applications.argoproj.io` and
`applicationsets.argoproj.io` from the cluster that runs Argo CD. No code
changed.

## Decisions recorded

1. kubectl over the Argo CD API. Both return the same CRD objects. kubectl
   needs no second credential, reuses the Kubernetes pack's cluster record
   and command source, and shares the redaction path. The API (through
   `#HTTPObserve` with a `can-i` guard) is a follow-up for a central Argo
   CD whose clusters pudl cannot reach.
2. Allow-list transforms shape `argocd.#Application` and
   `argocd.#ApplicationSet`; `spec.source` and `spec.sources` become one
   `sources` list with `index` and `syncRevision` per element; history
   entries drop their embedded sources.
3. Redaction with `sensitive_fields`: `helm.values`, `helm.valuesObject`,
   `helm.fileParameters[*].path`, and parameter values except an allow
   list of image parameters (`_argocdImageParameters`, default
   `image.name`, `image.tag`, `image.repository`), so Image Updater
   `argocd` write-back overrides stay visible. `kustomize.images` is kept.
4. Destination: `argocd_app_target_host` rule maps
   `https://kubernetes.default.svc` to the record's own `cluster`, and any
   other server to its URL host (`argocd_app_destination_host` fact).
   Destination by name is a follow-up (cluster Secrets).
5. Edges: `in_cluster`, `generated_by`, `deploys_to`, `deployed_by` (join on
   cluster, namespace, kind, name; one rule per workload source, with a new
   `gcp_gke_cluster_endpoint` fact for GCP records). `sourced_from` is
   declared but waits for a repository node, a core decision shared with
   `built_from` in `pudl-avqx.5`.

## Public API (planned, not built)

Facts: `argocd_app`, `argocd_app_source`, `argocd_app_destination`,
`argocd_app_destination_host`, `argocd_app_resource`, `argocd_app_image`,
`argocd_app_history`, `argocd_app_kustomize_image`,
`argocd_app_helm_parameter`, `argocd_app_image_updater`,
`argocd_app_owner`, `argocd_appset`, `argocd_appset_git_generator`.
Rule: `argocd_app_target_host`. Settings: `_argocdNamespaces`,
`_argocdImageParameters`.

## Follow-ups named in the doc

Repository node (core), destination by name, Argo CD API collectors. Build
is `pudl-avqx.4`, blocked by `pudl-3q5.1`.
