package argocd

import "list"

// An Argo CD ApplicationSet (applicationsets.argoproj.io), as the
// collector's allow-list transform shapes it: metadata, goTemplate and the
// sync policy; the template, whose spec goes through the Application allow
// list and sources normalization (template strings such as
// {{path.basename}} are kept as text); a summary of each generator instead
// of the generator; and the status conditions. Generator template
// overrides, list elements, templatePatch and every other field are
// dropped. The template's Helm values and non-image parameter values are
// redacted, as an Application's are.
#ApplicationSet: {
	_pudl: {
		schema_type:   "base"
		resource_type: "argocd.applicationset"
		identity_fields: ["cluster", "metadata.namespace", "metadata.name"]
		review: status: "draft"
		graph: name: "metadata.name"
		graph: display: {code: "APPSET", label: "Argo CD application set", class: "compute"}
		sensitive_fields: list.Concat([(_argocdSourceSensitive & {_prefix: "template.spec."}).out, [_argocdLastApplied]])
		same_resource: [{kind: "k8s.object", key: uid: "metadata.uid"}]
		facts: {
			// uid joins an Application's owner reference (generated_by).
			argocd_appset: args: {
				cluster:         "cluster"
				namespace:       "metadata.namespace"
				name:            "metadata.name"
				uid:             "metadata.uid"
				generator_types: "generatorTypes"
			}
			// Git generators at the top level, and one level down in a
			// matrix or merge generator.
			argocd_appset_git_generator: {
				each: "generators[*]"
				args: _argocdGitGeneratorArgs
			}
			argocd_appset_child_git_generator: {
				each: "generators[*].generators[*]"
				args: _argocdGitGeneratorArgs
			}
		}
	}
	apiVersion: =~"^argoproj\\.io/"
	kind:       "ApplicationSet" @pudl(match)
	cluster:    string & !=""
	metadata: {name: string, namespace: string, uid: string, ...}
	goTemplate?: bool
	syncPolicy?: {...}
	template: {
		metadata?: {...}
		spec: _#ArgoSpec
	}
	generators: [..._#ArgoGenerator]
	// The top-level generators' types, comma-separated; a matrix or merge
	// lists its children: matrix(git,list).
	generatorTypes: string
	status?: {...}
}

// _#ArgoGenerator is a generator's summary: its type (the one key the
// generator has), the git generator's repository, revision and path
// patterns, a list generator's element count, a clusters generator's
// selector, and a matrix or merge generator's children, one level deep.
_#ArgoGenerator: {
	type: string
	git?: {
		repoURL?:  string
		revision?: string
		directories?: [...{path: string, exclude?: bool}]
		files?: [...{path: string}]
	}
	list?: elementCount: int
	clusters?: selector?: {...}
	generators?: [..._#ArgoGenerator]
}

_argocdGitGeneratorArgs: {
	repo_url: {path: "git.repoURL", rewrite: _argocdPresent}
	revision: {path: "git.revision", default: ""}
}
