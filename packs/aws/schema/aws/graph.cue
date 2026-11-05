package aws

// Graph labels (`_pudl: graph`, read by `pudl graph export` and the web
// explorer). Every collector adds AccountId and, for regional types, Region.
// Edges between resources are rules in schema/pudl/rules/aws_graph.cue.
_awsGraph: {
	account: "AccountId"
	region:  "Region"
}
