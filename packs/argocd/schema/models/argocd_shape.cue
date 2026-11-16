package models

import (
	"list"
	"regexp"
	"strings"
)

// Allow-list transforms for Argo CD objects (packs/argocd/README.md in the
// PUDL source). Each keeps the named fields and drops the rest, and every
// repoURL kept loses its userinfo (_argocdRepoURL). Redaction
// of Helm values and parameter values is the schemas' sensitive_fields,
// after these run.

// The destination server of an Application deployed to the cluster Argo CD
// runs in.
_argocdInClusterServer: "https://kubernetes.default.svc"

// _argocdPick copies the fields of _src named in _keys that are present.
_argocdPick: {
	_src: _
	_keys: [...string]
	out: {for k in _keys if _src[k] != _|_ {(k): _src[k]}}
}

// _argocdRepoURLPattern splits a URL with a scheme into the scheme, the
// userinfo before the last "@" of the authority, if any, and the rest.
_argocdRepoURLPattern: "^([A-Za-z][A-Za-z0-9+.-]*)://(?:([^/?#]*)@)?(.*)$"

// _argocdRepoURL is a repository URL without the credentials it can carry
// as userinfo: https://user:TOKEN@host/path becomes https://host/path. An
// ssh:// URL keeps the user name (ssh://git@host/path) and drops a
// password. A URL without a scheme, such as the scp form git@host:org/repo,
// holds no secret and is kept as it is, as is a URL without userinfo.
_argocdRepoURL: {
	_url: string
	let u = _url
	out: string
	if u !~ _argocdRepoURLPattern {out: u}
	if u =~ _argocdRepoURLPattern {
		let m = regexp.FindSubmatch(_argocdRepoURLPattern, u)
		let user = strings.Split(m[2], ":")[0]
		let keep = [if strings.ToLower(m[1]) == "ssh" && user != "" {user + "@"}, ""][0]
		out: m[1] + "://" + keep + m[3]
	}
}

// _argocdMetadata keeps an object's metadata without managedFields.
_argocdMetadata: {
	_src: _
	let s = _src
	out: (_argocdPick & {_src: s, _keys: [
		"name", "namespace", "uid", "resourceVersion", "creationTimestamp",
		"labels", "annotations", "ownerReferences", "finalizers",
	]}).out
}

// _argocdHelm keeps a source's Helm block. values, valuesObject, file
// parameter paths and parameter values are kept here and redacted by the
// schema; passCredentials, namespace, kubeVersion and apiVersions are
// dropped. Each parameter carries its source's index (sourceIndex) for
// the argocd_app_helm_parameter fact.
_argocdHelm: {
	_src: _
	_index: int
	let s = _src
	let i = _index
	out: {
		(_argocdPick & {_src: s, _keys: [
			"valueFiles", "releaseName", "version", "skipCrds", "ignoreMissingValueFiles",
			"values", "valuesObject",
		]}).out
		if s.parameters != _|_ {
			parameters: [for p in s.parameters {
				sourceIndex: i
				(_argocdPick & {_src: p, _keys: ["name", "value", "forceString"]}).out
			}]
		}
		if s.fileParameters != _|_ {
			fileParameters: [for p in s.fileParameters {(_argocdPick & {_src: p, _keys: ["name", "path"]}).out}]
		}
	}
}

// _argocdSource keeps one source, at position _index. _sync is
// status.sync, or {} for a template; _multi says the spec used sources.
_argocdSource: {
	_src: _
	_index: int
	_multi: bool
	_sync: _
	let s = _src
	let i = _index
	out: {
		index: i
		(_argocdPick & {_src: s, _keys: ["path", "chart", "targetRevision", "ref", "name"]}).out
		if s.repoURL != _|_ {repoURL: (_argocdRepoURL & {_url: s.repoURL}).out}
		if s.helm != _|_ {helm: (_argocdHelm & {_src: s.helm, _index: i}).out}
		if s.kustomize != _|_ {
			kustomize: (_argocdPick & {_src: s.kustomize, _keys: ["images", "namePrefix", "nameSuffix", "namespace", "commonLabels"]}).out
		}
		if s.directory != _|_ {
			directory: (_argocdPick & {_src: s.directory, _keys: ["recurse", "include", "exclude"]}).out
		}
		// plugin.env and plugin.parameters can hold anything: name only.
		if s.plugin != _|_ {
			plugin: (_argocdPick & {_src: s.plugin, _keys: ["name"]}).out
		}
		if _multi && _sync.revisions[i] != _|_ {syncRevision: _sync.revisions[i]}
		if !_multi && _sync.revision != _|_ {syncRevision: _sync.revision}
	}
}

// _argocdHost is a server URL's host: its scheme, a trailing slash and a
// :443 port removed, as the Kubernetes pack's cluster helper computes its
// own host.
_argocdHost: {
	_server: string
	out: strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(_server, "https://"), "http://"), "/"), ":443")
}

// _argocdSpec keeps the parts of an Application spec facts need: project,
// sources (spec.sources, or [spec.source]), destination and sync policy.
// For an Application (_app), destinationHost is the destination server's
// host unless it is the in-cluster server.
_argocdSpec: {
	_src: _
	_sync: _
	_app: bool
	let sp = _src
	let sy = _sync
	let multi = sp.sources != _|_
	let raw = [if multi {sp.sources}, if sp.source != _|_ {[sp.source]}, []][0]
	out: {
		if sp.project != _|_ {project: sp.project}
		sources: [for i, s in raw {(_argocdSource & {_src: s, _index: i, _multi: multi, _sync: sy}).out}]
		if sp.destination != _|_ {
			destination: (_argocdPick & {_src: sp.destination, _keys: ["server", "name", "namespace"]}).out
			if _app && sp.destination.server != _|_ {
				if sp.destination.server != _argocdInClusterServer {
					destinationHost: (_argocdHost & {_server: sp.destination.server}).out
				}
			}
		}
		if sp.syncPolicy != _|_ {
			syncPolicy: {
				if sp.syncPolicy.automated != _|_ {
					automated: (_argocdPick & {_src: sp.syncPolicy.automated, _keys: ["enabled", "prune", "selfHeal"]}).out
				}
				if sp.syncPolicy.syncOptions != _|_ {syncOptions: sp.syncPolicy.syncOptions}
			}
		}
	}
}

// _argocdStatus keeps an Application's status fields facts and the
// inspector need. operationState keeps the phase, times, the revision and
// who started it, not the sync result; history entries drop their
// source and sources, which repeat the Helm block of each past sync.
_argocdStatus: {
	_src: _
	let st = _src
	out: {
		if st.sync != _|_ {sync: (_argocdPick & {_src: st.sync, _keys: ["status", "revision", "revisions"]}).out}
		if st.health != _|_ {health: (_argocdPick & {_src: st.health, _keys: ["status"]}).out}
		(_argocdPick & {_src: st, _keys: ["reconciledAt", "sourceType", "sourceTypes"]}).out
		if st.operationState != _|_ {
			let op = st.operationState
			operationState: {
				(_argocdPick & {_src: op, _keys: ["phase", "startedAt", "finishedAt"]}).out
				if op.operation != _|_ {
					operation: {
						if op.operation.sync.revision != _|_ {sync: revision: op.operation.sync.revision}
						if op.operation.initiatedBy != _|_ {
							initiatedBy: (_argocdPick & {_src: op.operation.initiatedBy, _keys: ["username", "automated"]}).out
						}
					}
				}
			}
		}
		if st.conditions != _|_ {conditions: [for c in st.conditions {(_argocdPick & {_src: c, _keys: ["type", "message"]}).out}]}
		if st.resources != _|_ {
			resources: [for r in st.resources {
				(_argocdPick & {_src: r, _keys: ["group", "version", "kind", "namespace", "name", "status", "requiresPruning"]}).out
				if r.health.status != _|_ {health: status: r.health.status}
			}]
		}
		if st.summary != _|_ {summary: (_argocdPick & {_src: st.summary, _keys: ["images", "externalURLs"]}).out}
		if st.history != _|_ {
			history: [for h in st.history {
				(_argocdPick & {_src: h, _keys: ["id", "revision", "revisions", "deployStartedAt", "deployedAt"]}).out
				if h.initiatedBy != _|_ {initiatedBy: (_argocdPick & {_src: h.initiatedBy, _keys: ["username", "automated"]}).out}
			}]
		}
	}
}

// _argocdApplication is one argocd.#Application record from a kubectl
// item. cluster is set by the run.
_argocdApplication: {
	_in: _
	let appSpec = *_in.spec | {}
	let st = *_in.status | {}
	apiVersion: _in.apiVersion
	kind:       _in.kind
	metadata:   (_argocdMetadata & {_src: _in.metadata}).out
	(_argocdSpec & {_src: appSpec, _sync: *st.sync | {}, _app: true}).out
	if _in.status != _|_ {status: (_argocdStatus & {_src: st}).out}
}

// The generator types an ApplicationSet can use.
_argocdGeneratorTypes: ["git", "list", "clusters", "pullRequest", "scmProvider", "matrix", "merge", "plugin", "clusterDecisionResource"]

// _argocdGeneratorLeaf summarizes one generator without its children.
// list elements, generator template overrides, clusters values, SCM and
// pull request credentials and plugin input are dropped; a git
// generator's repoURL loses its userinfo.
_argocdGeneratorLeaf: {
	_src: _
	let g = _src
	let types = [for k, _ in g if list.Contains(_argocdGeneratorTypes, k) {k}]
	out: {
		type: [if len(types) > 0 {types[0]}, "unknown"][0]
		if g.git != _|_ {
			git: {
				(_argocdPick & {_src: g.git, _keys: ["revision"]}).out
				if g.git.repoURL != _|_ {repoURL: (_argocdRepoURL & {_url: g.git.repoURL}).out}
				if g.git.directories != _|_ {
					directories: [for d in g.git.directories {(_argocdPick & {_src: d, _keys: ["path", "exclude"]}).out}]
				}
				if g.git.files != _|_ {
					files: [for f in g.git.files {(_argocdPick & {_src: f, _keys: ["path"]}).out}]
				}
			}
		}
		if g.list != _|_ {
			let elements = *g.list.elements | []
			list: elementCount: len(elements)
		}
		if g.clusters.selector != _|_ {clusters: selector: g.clusters.selector}
	}
}

// _argocdGenerator summarizes a generator and, for a matrix or merge, its
// children one level deep.
_argocdGenerator: {
	_src: _
	let g = _src
	let leaf = (_argocdGeneratorLeaf & {_src: g}).out
	let parent = [if g.matrix != _|_ {g.matrix}, if g.merge != _|_ {g.merge}, {}][0]
	let childSrc = *parent.generators | []
	let children = [for c in childSrc {(_argocdGeneratorLeaf & {_src: c}).out}]
	out: {
		leaf
		if len(children) > 0 {generators: children}
	}
	label: [
		if len(children) > 0 {"\(leaf.type)(\(strings.Join([for c in children {c.type}], ",")))"},
		leaf.type,
	][0]
}

// _argocdApplicationSet is one argocd.#ApplicationSet record from a kubectl
// item. cluster is set by the run.
_argocdApplicationSet: {
	_in: _
	let appSpec = *_in.spec | {}
	let tmpl = *appSpec.template | {}
	let gensSrc = *appSpec.generators | []
	let gens = [for g in gensSrc {_argocdGenerator & {_src: g}}]
	apiVersion: _in.apiVersion
	kind:       _in.kind
	metadata:   (_argocdMetadata & {_src: _in.metadata}).out
	if appSpec.goTemplate != _|_ {goTemplate: appSpec.goTemplate}
	if appSpec.syncPolicy != _|_ {syncPolicy: appSpec.syncPolicy}
	template: {
		if tmpl.metadata != _|_ {
			metadata: (_argocdPick & {_src: tmpl.metadata, _keys: ["name", "labels", "annotations"]}).out
		}
		spec: (_argocdSpec & {_src: *tmpl.spec | {}, _sync: {}, _app: false}).out
	}
	generators: [for g in gens {g.out}]
	generatorTypes: strings.Join([for g in gens {g.label}], ",")
	if _in.status.conditions != _|_ {
		status: conditions: [for c in _in.status.conditions {(_argocdPick & {_src: c, _keys: ["type", "message"]}).out}]
	}
}
