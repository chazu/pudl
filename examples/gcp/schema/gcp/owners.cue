package gcp

// Ownership. Every type declares `_pudl: owner`: a resource belongs to the
// team in its `team` label, else to its project's team in _gcpProjectOwners,
// else to nobody. Unowned resources are the resource_unowned relation
// (`pudl query resource_unowned`); `pudl check --owner TEAM` shows one team's
// findings.
//
// Name the team of each project you collect:
//
//	_gcpProjectOwners: {"my-project": "platform", "data-project": "data"}
_gcpProjectOwners: {[string]: string}

// _#GCPOwner is the owner declaration the types share. Types with labels add
// their label path: _#GCPOwner & {paths: ["labels.team"]}.
_#GCPOwner: {
	paths?: [...string]
	map: {path: "project", owners: _gcpProjectOwners}
}
