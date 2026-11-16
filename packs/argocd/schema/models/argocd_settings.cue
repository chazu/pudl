package models

// Argo CD pack settings. Override them in another file of package models,
// e.g. .pudl/schema/settings.cue. The image parameter allow list is a
// schema setting: see .pudl/schema/argocd/redaction.cue.

// The namespaces whose Applications and ApplicationSets the collectors
// keep. The collectors list every namespace (Argo CD can watch
// Applications in any namespace); this filters what they store. An empty
// list keeps every namespace:
//
//	_argocdNamespaces: []
_argocdNamespaces: [...string] | *["argocd"]
