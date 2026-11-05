package gcp

// Graph labels (`_pudl: graph`, read by `pudl graph export` and the web
// explorer). Every collector sets project. Compute records name each other
// by selfLink with the API prefix trimmed (projects/P/regions/R/...), which
// is how the pack's facts store references, so compute types declare that
// form as a ref. Edges are rules in schema/pudl/rules/gcp_graph.cue.
// name is a default so a type can name another field or path form.
_gcpGraph: {
	name:    *"name" | _
	account: "project"
}

_gcpComputeGraph: _gcpGraph & {
	refs: [{path: "selfLink", trim_prefix: _gcpComputeAPI}]
}
