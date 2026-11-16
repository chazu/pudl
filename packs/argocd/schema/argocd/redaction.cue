package argocd

// Settings and redaction shared by the two schemas.

// The Helm parameters whose values pudl keeps. Argo CD Image Updater writes
// the running image into these with the argocd write-back method, so the
// tag lives only in the cluster. Every other parameter's value is redacted;
// its name stays. A workspace whose Image Updater alias annotations name
// other parameters (<alias>.helm.image-name, <alias>.helm.image-tag) sets
// this in another file of package argocd, e.g.
// .pudl/schema/argocd/settings.cue:
//
//	package argocd
//
//	_argocdImageParameters: ["image.name", "image.tag", "image.repository", "proxy.image.tag"]
_argocdImageParameters: [...string] | *["image.name", "image.tag", "image.repository"]

// _argocdSourceSensitive lists the Helm paths of a normalized sources list
// that can hold credentials, under prefix: inline values, valuesObject, the
// file a file parameter reads, and the value of every parameter not on the
// image allow list.
_argocdSourceSensitive: {
	_prefix: string
	out: [
		"\(_prefix)sources[*].helm.values",
		"\(_prefix)sources[*].helm.valuesObject",
		"\(_prefix)sources[*].helm.fileParameters[*].path",
		{
			each:  "\(_prefix)sources[*].helm.parameters[*]"
			value: "value"
			keep_when: {field: "name", in: _argocdImageParameters}
		},
	]
}

// kubectl apply records the whole object, Helm values included, in this
// annotation.
_argocdLastApplied: #"metadata.annotations."kubectl.kubernetes.io/last-applied-configuration""#

// A rewrite that accepts any non-empty string, so an absent field yields
// no fact rather than a fact without the arg.
_argocdPresent: [{match: "^(.+)$", replace: "$1"}]
