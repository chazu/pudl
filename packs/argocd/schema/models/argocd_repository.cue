package models

import (
	"list"
	"regexp"
	"strings"
)

// Repositories (docs/design/2026-11-13-argocd-pack.md, decision 7 and
// section 7 in the PUDL source). A repository is a record,
// argocd.#Repository, one per repository URL the Applications and
// ApplicationSets name, identified by the normalized URL (_argocdRepoID).
// A git repository's record holds the git.repo key, so another pack's
// record of the same repository is the same graph node.

// _argocdRepoIDURLPattern splits a URL with a scheme into the scheme, the
// host with its port, and the path with any query and fragment; userinfo
// is skipped.
_argocdRepoIDURLPattern: "(?s)^([A-Za-z][A-Za-z0-9+.-]*)://(?:[^/?#]*@)?([^/?#]*)(.*)$"

// _argocdRepoIDSCPPattern is the scp form git@host:org/repo: a user, the
// host and the path. The user is required, so host:port/path is not one.
_argocdRepoIDSCPPattern: "(?s)^[^@/:]+@([^/:]+):(.*)$"

// The schemes that reach a repository over ssh. The repository is the same
// one its https URL names.
_argocdRepoSSHSchemes: ["ssh", "git", "git+ssh", "ssh+git"]

// _argocdRepoPath is a repository URL's path without the query, the
// fragment, a trailing slash and a trailing .git.
_argocdRepoPath: {
	_path: string
	let p = strings.Split(strings.Split(_path, "#")[0], "?")[0]
	out: strings.TrimRight(strings.TrimSuffix(strings.TrimRight(p, "/"), ".git"), "/")
}

// _argocdRepoID is the repository a repoURL names: the identity of an
// argocd.#Repository record and its git.repo key. The URL first loses its
// userinfo (_argocdRepoURL). Then the ssh schemes and the scp form become
// https without a port, the host is lower-cased, a default port is
// dropped, and the path loses its query, fragment, trailing slash and
// .git. git@github.com:org/repo.git, ssh://git@github.com/org/repo.git and
// https://GitHub.com/org/repo/ are all https://github.com/org/repo. A value
// without a scheme that is not the scp form keeps no scheme.
_argocdRepoID: {
	_url: string
	let raw = _url
	let u = strings.TrimSpace((_argocdRepoURL & {_url: raw}).out)
	out: string
	if u =~ _argocdRepoIDURLPattern {
		let m = regexp.FindSubmatch(_argocdRepoIDURLPattern, u)
		let scheme = strings.ToLower(m[1])
		let ssh = list.Contains(_argocdRepoSSHSchemes, scheme)
		let hp = strings.ToLower(m[2])
		let host = [
			if ssh {strings.Split(hp, ":")[0]},
			if scheme == "https" {strings.TrimSuffix(hp, ":443")},
			if scheme == "http" {strings.TrimSuffix(hp, ":80")},
			hp,
		][0]
		out: [if ssh {"https"}, scheme][0] + "://" + host + (_argocdRepoPath & {_path: m[3]}).out
	}
	if u !~ _argocdRepoIDURLPattern && u =~ _argocdRepoIDSCPPattern {
		let m = regexp.FindSubmatch(_argocdRepoIDSCPPattern, u)
		out: "https://" + strings.ToLower(m[1]) + (_argocdRepoPath & {_path: "/" + strings.TrimPrefix(m[2], "/")}).out
	}
	if u !~ _argocdRepoIDURLPattern && u !~ _argocdRepoIDSCPPattern {
		let host = strings.Split(u, "/")[0]
		out: strings.ToLower(host) + (_argocdRepoPath & {_path: strings.TrimPrefix(u, host)}).out
	}
}

// _argocdRepoFields are the repoURL of a source or git generator without
// its userinfo, and repo, the repository it names (_argocdRepoID). A
// repoURL that holds a template ({{.repoURL}}) names no repository yet.
_argocdRepoFields: {
	_url: string
	let raw = _url
	out: {
		repoURL: (_argocdRepoURL & {_url: raw}).out
		if !strings.Contains(raw, "{{") {repo: (_argocdRepoID & {_url: raw}).out}
	}
}

// _argocdRepoRefs lists the repositories one shaped record names, each as
// {url, type}: an Application's sources, an ApplicationSet's template
// sources, and its git generators at the top level and inside a matrix or
// merge. A source that names a chart is a Helm repository, an oci:// URL
// without a chart an OCI one, and every other source and git generator a
// git repository.
_argocdRepoRefs: {
	_rec: _
	let r = _rec
	let sources = list.Concat([*r.sources | [], *r.template.spec.sources | []])
	let gens = *r.generators | []
	let gits = list.Concat([
		[for g in gens if g.git.repo != _|_ {g.git}],
		[for g in gens if g.generators != _|_ for c in g.generators if c.git.repo != _|_ {c.git}],
	])
	out: list.Concat([
		[for s in sources if s.repo != _|_ {
			url: s.repo
			type: [if s.chart != _|_ {"helm"}, if strings.HasPrefix(s.repo, "oci://") {"oci"}, "git"][0]
		}],
		[for g in gits {url: g.repo, type: "git"}],
	])
}

// _argocdRepository is one argocd.#Repository record: url, the host and
// path as name, and type. Only a git repository has gitURL, its git.repo
// key: a Helm chart repository is not a git repository.
_argocdRepository: {
	_url:  string
	_type: string
	url:   _url
	name:  regexp.ReplaceAll("^[a-z][a-z0-9+.-]*://", _url, "")
	type:  _type
	if _type == "git" {gitURL: _url}
}

// _argocdRepositories is one record per repository in _refs, sorted by URL.
// A URL some source uses as git and another as a chart is git.
_argocdRepositories: {
	_refs: [...{url: string, type: string}]
	let refs = _refs
	let urls = list.SortStrings([for u, _ in {for r in refs {(r.url): true}} {u}])
	out: [for u in urls {
		let types = [for r in refs if r.url == u {r.type}]
		_argocdRepository & {_url: u, _type: [if list.Contains(types, "git") {"git"}, types[0]][0]}
	}]
}

// _argocdRepoListTransform reads the List `kubectl get -o json` prints,
// shapes each item kept (_argocdKept) with _shape, and stores one
// repository record per repository the items name.
_argocdRepoListTransform: {
	_shape: _
	in:     _
	let shape = _shape
	let kept = [for item in in.items if (_argocdKept & {_ns: item.metadata.namespace}).ok {shape & {_in: item}}]
	out: (_argocdRepositories & {_refs: list.Concat([for r in kept {(_argocdRepoRefs & {_rec: r}).out}])}).out
}
