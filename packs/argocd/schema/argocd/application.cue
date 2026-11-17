package argocd

// An Argo CD Application (applications.argoproj.io), as the collector's
// allow-list transform (schema/models/argocd_pack.cue) shapes it from
// `kubectl get -o json`: metadata, the spec's project, sources, destination
// and sync policy at the top level, and the status fields facts need.
// spec.source and spec.sources become one sources list; plugin env and
// parameters, ignoreDifferences, the history entries' sources, the
// sync result and every annotation but Argo CD's and Image Updater's are
// dropped. Helm values and every parameter value except
// the image parameters are redacted before anything is stored.
//
// cluster is the host of the cluster the record was collected from (the
// Kubernetes pack's cluster record), where Argo CD runs; destination is
// where the Application deploys.
#Application: {
	_pudl: {
		schema_type:   "base"
		resource_type: "argocd.application"
		identity_fields: ["cluster", "metadata.namespace", "metadata.name"]
		review: status: "draft"
		graph: name: "metadata.name"
		graph: display: {code: "APP", label: "Argo CD application", class: "compute"}
		sensitive_fields: (_argocdSourceSensitive & {_prefix: ""}).out
		same_resource: [{kind: "k8s.object", key: uid: "metadata.uid"}]
		facts: {
			argocd_app: args: {
				cluster:       "cluster"
				namespace:     "metadata.namespace"
				name:          "metadata.name"
				project:       "project"
				sync_status:   "status.sync.status"
				health_status: "status.health.status"
				// Single source only; a multi-source app's revisions are
				// on argocd_app_source.
				sync_revision: "status.sync.revision"
				reconciled_at: "status.reconciledAt"
				automated: {exists: "syncPolicy.automated"}
				self_heal: {path: "syncPolicy.automated.selfHeal", default: false}
				prune: {path: "syncPolicy.automated.prune", default: false}
			}
			argocd_app_source: {
				each: "sources[*]"
				args: {
					index:           "index"
					repo_url:        "repoURL"
					// The repository's normalized URL: the url of its
					// argocd.#Repository record (sourced_from).
					repo:            "repo"
					path:            "path"
					chart:           "chart"
					target_revision: "targetRevision"
					ref:             "ref"
					sync_revision:   "syncRevision"
				}
			}
			argocd_app_destination: args: {
				server: {path: "destination.server", default: ""}
				name: {path: "destination.name", default: ""}
				namespace: {path: "destination.namespace", default: ""}
			}
			// The destination server's host, when it is not the in-cluster
			// server. A destination given by name has none.
			argocd_app_destination_host: args: host: {path: "destinationHost", rewrite: _argocdPresent}
			argocd_app_resource: {
				each: "status.resources[*]"
				args: {
					group: {path: "group", default: ""}
					version: {path: "version", default: ""}
					kind: "kind"
					namespace: {path: "namespace", default: ""}
					name: "name"
					status: {path: "status", default: ""}
					health: {path: "health.status", default: ""}
				}
			}
			argocd_app_image: args: image: "status.summary.images[*]"
			argocd_app_history: {
				each: "status.history[*]"
				args: {
					id: "id"
					revision: {path: "revision", default: ""}
					deployed_at: "deployedAt"
					automated: {path: "initiatedBy.automated", default: false}
				}
			}
			argocd_app_kustomize_image: {
				each: "sources[*]"
				args: {index: "index", image: "kustomize.images[*]"}
			}
			// value is "[REDACTED]" unless the name is on
			// _argocdImageParameters.
			argocd_app_helm_parameter: {
				each: "sources[*].helm.parameters[*]"
				args: {
					index: "sourceIndex"
					name:  "name"
					value: {path: "value", default: ""}
				}
			}
			argocd_app_image_updater: args: {
				image_list: {path: #"metadata.annotations."argocd-image-updater.argoproj.io/image-list""#, rewrite: _argocdPresent}
				write_back_method: {path: #"metadata.annotations."argocd-image-updater.argoproj.io/write-back-method""#, default: "argocd"}
				write_back_target: {path: #"metadata.annotations."argocd-image-updater.argoproj.io/write-back-target""#, default: ""}
			}
			argocd_app_owner: {
				each: "metadata.ownerReferences[*]"
				args: {kind: "kind", name: "name", uid: "uid"}
			}
		}
	}
	apiVersion: =~"^argoproj\\.io/"
	kind:       "Application" @pudl(match)
	cluster:    string & !=""
	metadata: {name: string, namespace: string, uid: string, ...}
	_#ArgoSpec
	status?: {...}
}

// _#ArgoSpec is the part of an Application spec the allow list keeps, as an
// Application record and an ApplicationSet template hold it.
_#ArgoSpec: {
	project?: string
	sources: [..._#ArgoSource]
	destination?: {server?: string, name?: string, namespace?: string}
	// destination.server's host (scheme, a :443 port and a trailing slash
	// removed), unless it is the in-cluster server. Applications only.
	destinationHost?: string
	syncPolicy?: {...}
	...
}

// _#ArgoSource is one element of the sources list. index is its position;
// repo is the normalized repoURL, the url of the source's argocd.#Repository
// record, absent when repoURL holds a template; syncRevision is the
// revision Argo CD last synced it to.
_#ArgoSource: {
	index:           int
	repoURL?:        string
	repo?:           string
	path?:           string
	chart?:          string
	targetRevision?: string
	ref?:            string
	name?:           string
	syncRevision?:   string
	helm?: {
		valueFiles?: [...string]
		// "[REDACTED]" once stored.
		values?:       _
		valuesObject?: _
		parameters?: [...{name: string, value?: _, sourceIndex: int, ...}]
		fileParameters?: [...{...}]
		...
	}
	kustomize?: {images?: [...string], ...}
	directory?: {...}
	plugin?: {name?: string}
}
