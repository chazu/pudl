package argocd

// A repository an Argo CD Application or ApplicationSet names: a source's
// repoURL, or a git generator's. The argocd-repositories collector writes
// one record per repository (schema/models/argocd_repository.cue), so the
// sourced_from edge ends at a node a record backs
// (docs/design/2026-11-13-argocd-pack.md, decision 7, in the PUDL source).
//
// url, the identity, is the normalized repoURL: no userinfo, the ssh
// schemes and the scp form (git@host:org/repo) as https without a port, a
// lower-case host without a default port, and no query, fragment,
// trailing slash or .git. name is url without its scheme.
//
// type is helm for a source that names a chart, oci for an oci:// URL
// without a chart, and git otherwise. Only a git repository has gitURL,
// which is its git.repo key: another pack's record of the same git
// repository, keyed the same way, is the same graph node.
#Repository: {
	_pudl: {
		schema_type:   "base"
		resource_type: "argocd.repository"
		identity_fields: ["url"]
		review: status: "draft"
		graph: name: "name"
		graph: display: {code: "REPO", label: "Repository", class: "data"}
		same_resource: [{kind: "git.repo", key: url: "gitURL"}]
		facts: argocd_repo: args: {
			url:  "url"
			name: "name"
			type: "type"
		}
	}
	url:     string & !=""
	name:    string & !=""
	type:    "git" | "helm" | "oci"
	gitURL?: string & !=""
}
