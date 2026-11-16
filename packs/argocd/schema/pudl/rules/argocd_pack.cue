package rules

// Argo CD content pack rules. Relations come from the pack's fact
// projection (argocd.#Application, argocd.#ApplicationSet), the Kubernetes
// pack's k8s_cluster, and the GCP pack's gcp_k8s_deployment,
// gcp_gke_cluster, gcp_gke_cluster_endpoint and gcp_k8s_deployment_argo_app.
// A rule over a pack that is not installed derives nothing.

// argocd_app_target_host — the host of the cluster an Application deploys
// to. The in-cluster server is the cluster the record came from; any other
// server is its URL's host (argocd_app_destination_host). A destination
// given by name has no host: resolving a name needs Argo CD's cluster
// Secrets, which pudl does not read.
argocd_app_target_host_in_cluster: {
	head: {rel: "argocd_app_target_host", args: {resource_id: "$R", host: "$H"}}
	body: [
		{rel: "argocd_app", args: {resource_id: "$R", cluster: "$H"}},
		{rel: "argocd_app_destination", args: {resource_id: "$R", server: "https://kubernetes.default.svc"}},
	]
}
argocd_app_target_host_server: {
	head: {rel: "argocd_app_target_host", args: {resource_id: "$R", host: "$H"}}
	body: [{rel: "argocd_app_destination_host", args: {resource_id: "$R", host: "$H"}}]
}

// argocd_cluster — a cluster record by a host its API server answers on: the
// Kubernetes pack's cluster record, and a GKE cluster's public or DNS
// endpoint. A cluster with both is one same_resource group.
argocd_cluster_k8s: {
	head: {rel: "argocd_cluster", args: {resource_id: "$C", host: "$H"}}
	body: [{rel: "k8s_cluster", args: {resource_id: "$C", host: "$H"}}]
}
argocd_cluster_gke: {
	head: {rel: "argocd_cluster", args: {resource_id: "$C", host: "$H"}}
	body: [{rel: "gcp_gke_cluster_endpoint", args: {resource_id: "$C", host: "$H"}}]
}

// argocd_workload_app — a workload an Application deploys: Argo CD lists it
// among the Application's managed resources (kind, namespace and name), and
// the workload lives in the cluster the Application deploys to. This is the
// basis of the deployed_by edge; status.resources has no UID.
//
// GCP pack deployments (gcp.#K8sDeployment): the cluster is matched through
// the GKE cluster's endpoints.
argocd_workload_app_gcp_deployment: {
	head: {rel: "argocd_workload_app", args: {workload: "$W", application: "$A"}}
	body: [
		{rel: "gcp_k8s_deployment", args: {resource_id: "$W", project: "$P", location: "$L", cluster: "$N", namespace: "$NS", name: "$D"}},
		{rel: "gcp_gke_cluster", args: {resource_id: "$C", project: "$P", location: "$L", name: "$N"}},
		{rel: "gcp_gke_cluster_endpoint", args: {resource_id: "$C", host: "$H"}},
		{rel: "argocd_app_target_host", args: {resource_id: "$A", host: "$H"}},
		{rel: "argocd_app_resource", args: {resource_id: "$A", group: "apps", kind: "Deployment", namespace: "$NS", name: "$D"}},
	]
}

// argocd_workload_app_confirmed — a deployed_by join the workload confirms:
// its instance label or its tracking-id annotation names the same
// Application (via says which). A label can be set by hand, so it checks the
// join and is not its basis. With the tracking-id annotation the name is
// the Application's own in Argo CD's namespace; an Application elsewhere is
// <namespace>_<name> there and does not confirm.
argocd_workload_app_confirmed: {
	head: {rel: "argocd_workload_app_confirmed", args: {workload: "$W", application: "$A", via: "$V"}}
	body: [
		{rel: "argocd_workload_app", args: {workload: "$W", application: "$A"}},
		{rel: "argocd_app", args: {resource_id: "$A", name: "$N"}},
		{rel: "gcp_k8s_deployment_argo_app", args: {resource_id: "$W", application: "$N", via: "$V"}},
	]
}

// argocd_app_last_deploy — when an Application last finished a sync, from
// its history.
argocd_app_last_deploy: {
	head: {rel: "argocd_app_last_deploy", args: {resource_id: "$R", deployed_at: "max($D)"}}
	body: [{rel: "argocd_app_history", args: {resource_id: "$R", deployed_at: "$D"}}]
}

// ---- Inspector details (graph_detail), section "Argo CD", on an
// Application and on every workload it deploys. Every rule of a relation
// has the same head keys, so the rows with one value per Application carry
// rank 0 and the source rows their source's index.

argocd_detail_subject_app: {
	head: {rel: "argocd_detail_subject", args: {subject: "$R", application: "$R"}}
	body: [{rel: "argocd_app", args: {resource_id: "$R"}}]
}
argocd_detail_subject_workload: {
	head: {rel: "argocd_detail_subject", args: {subject: "$W", application: "$A"}}
	body: [{rel: "argocd_workload_app", args: {workload: "$W", application: "$A"}}]
}

argocd_detail_application: {
	head: {rel: "graph_detail", args: {resource_id: "$S", section: "Argo CD", key: "application", value: "$V", rank: 0}}
	body: [
		{rel: "argocd_detail_subject", args: {subject: "$S", application: "$A"}},
		{rel: "argocd_app", args: {resource_id: "$A", name: "$V"}},
	]
}
argocd_detail_sync_status: {
	head: {rel: "graph_detail", args: {resource_id: "$S", section: "Argo CD", key: "sync status", value: "$V", rank: 0}}
	body: [
		{rel: "argocd_detail_subject", args: {subject: "$S", application: "$A"}},
		{rel: "argocd_app", args: {resource_id: "$A", sync_status: "$V"}},
	]
}
argocd_detail_health: {
	head: {rel: "graph_detail", args: {resource_id: "$S", section: "Argo CD", key: "health", value: "$V", rank: 0}}
	body: [
		{rel: "argocd_detail_subject", args: {subject: "$S", application: "$A"}},
		{rel: "argocd_app", args: {resource_id: "$A", health_status: "$V"}},
	]
}
argocd_detail_last_deploy: {
	head: {rel: "graph_detail", args: {resource_id: "$S", section: "Argo CD", key: "last deploy", value: "$V", rank: 0}}
	body: [
		{rel: "argocd_detail_subject", args: {subject: "$S", application: "$A"}},
		{rel: "argocd_app_last_deploy", args: {resource_id: "$A", deployed_at: "$V"}},
	]
}
argocd_detail_write_back: {
	head: {rel: "graph_detail", args: {resource_id: "$S", section: "Argo CD", key: "image write-back", value: "$V", rank: 0}}
	body: [
		{rel: "argocd_detail_subject", args: {subject: "$S", application: "$A"}},
		{rel: "argocd_app_image_updater", args: {resource_id: "$A", write_back_method: "$V"}},
	]
}

// One row per source and field, ranked by the source's index. A field the
// source does not set has no row.
_argocdSourceDetails: {
	repo:              "repo_url"
	path:              "path"
	chart:             "chart"
	"target revision": "target_revision"
	"synced revision": "sync_revision"
}
for k, a in _argocdSourceDetails {
	"argocd_detail_source_\(a)": {
		head: {rel: "graph_detail", args: {resource_id: "$S", section: "Argo CD", key: k, value: "$V", rank: "$I"}}
		body: [
			{rel: "argocd_detail_subject", args: {subject: "$S", application: "$A"}},
			{rel: "argocd_app_source", args: {resource_id: "$A", index: "$I", (a): "$V"}},
		]
	}
}
