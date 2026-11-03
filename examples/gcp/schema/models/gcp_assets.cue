package models

import (
	"list"
	"strings"
)

// Cloud Asset Inventory (CAI) sources for the live collectors.
//
// A type listed in _gcpAssetTypes is collected with one CAI search per
// organization the credentials can see (or per project, with
// _gcpAssetScope: "projects"), and falls back to its gcloud list commands
// when CAI cannot answer (_gcpAssetUnavailable). Both sources pass through
// the type's shape in _gcpShapes, so either one stores identical records and
// switching between them creates no new versions. The settings
// (_gcpAssetScope, _gcpBillingProject) are in gcp_pack.cue.
//
// A type is listed only after its CAI copy was compared with gcloud's on
// live data and found to differ only in what its shape normalizes. Compute
// instances are not listed: CAI's copy of instance metadata omits most keys
// (on GKE nodes, all but a few, dropping settings such as
// disable-legacy-endpoints) and lower-cases values, so instances stay on
// gcloud.

// The CAI asset type of each type collected through CAI.
_gcpAssetTypes: {
	networks:  "compute.googleapis.com/Network"
	subnets:   "compute.googleapis.com/Subnetwork"
	firewalls: "compute.googleapis.com/Firewall"
	routes:    "compute.googleapis.com/Route"
	routers:   "compute.googleapis.com/Router"
	addresses: "compute.googleapis.com/Address"
}

// Errors that mean CAI cannot answer for these credentials: the asset API is
// disabled in the billing project or the caller may not bill it, or the
// caller may not search the scope. Any of them switches the collector to its
// gcloud commands for that run.
_gcpAssetUnavailable: [
	"SERVICE_DISABLED",
	"USER_PROJECT_DENIED",
	"PERMISSION_DENIED",
	"does not have permission to access",
]

// The organizations the credentials can see, as organizations/<id>.
_gcpOrgList: {
	argv: ["gcloud", "organizations", "list", "--format=json"]
	select: "name"
}

_gcpResourceManagerProject: "//cloudresourcemanager.googleapis.com/projects/"

// The CAI search populate of a type.
_gcpAssetSearch: {
	_t:         string
	let type = _t
	_assetType: _gcpAssetTypes[_t]
	_byProject: _gcpAssetScope == "projects"

	each: [if _byProject {_gcpProjectList}, _gcpOrgList][0]
	runs: [{
		argv: list.Concat([
			["gcloud", "asset", "search-all-resources",
				[if _byProject {"--scope=projects/{{item}}"}, "--scope={{item}}"][0],
				"--asset-types=\(_assetType)",
				"--read-mask=name,assetType,parentFullResourceName,versionedResources",
				"--page-size=500", "--format=json"],
			[if _gcpBillingProject != "" {"--billing-project=\(_gcpBillingProject)"}],
		])
		if _byProject {
			set: project: "{{item}}"
		}
		transform: _gcpAssetTransform & {_t: type}
	}]
}

// Unwraps a CAI search result into the type's shape. The project is the
// resource's parent project; other asset types are dropped.
_gcpAssetTransform: {
	_t: string
	in: _
	_resource: in.versionedResources[0].resource
	_shaped:   (_gcpShapes[_t] & {in: _resource}).out
	// The first element exists only for another asset type, so it selects
	// null; otherwise the shaped resource. (out must be declared outright:
	// a transform's in is unknown until a record arrives.)
	out: [
		if in.assetType != _gcpAssetTypes[_t] {null},
		_shaped & {project: strings.TrimPrefix(in.parentFullResourceName, _gcpResourceManagerProject)},
	][0]
}

// Puts a gcloud list record into the type's shape.
_gcpGcloudTransform: {
	_t:      string
	in:      _
	_record: in
	out:     (_gcpShapes[_t] & {in: _record}).out
}

// The shape each source is normalized to: the v1 API object as gcloud prints
// it, minus what differs between the two sources for the same resource.
_gcpShapes: {
	networks:  _gcpComputeShape & {_kind: "compute#network"}
	subnets:   _gcpComputeShape & {_kind: "compute#subnetwork"}
	firewalls: _gcpComputeShape & {_kind: "compute#firewall"}
	routes:    _gcpComputeShape & {_kind: "compute#route"}
	routers:   _gcpComputeShape & {_kind: "compute#router"}
	addresses: _gcpComputeShape & {_kind: "compute#address"}
}

// A Compute Engine list record. Compared on live data (2026-11-03), CAI's
// copy differs from gcloud's only in fields that are not configuration:
// CAI omits kind; fingerprint and labelFingerprint (optimistic-lock tokens)
// can differ; gcloud adds derived x_gcloud_* fields (e.g.
// x_gcloud_subnet_mode on networks) that no schema reads. The shape drops
// all of them and sets kind, which the schemas match on.
_gcpComputeShape: {
	_kind: string
	in: {...}
	let r = in
	out: {
		for k, v in r if !list.Contains(["kind", "fingerprint", "labelFingerprint"], k) && !strings.HasPrefix(k, "x_gcloud_") {
			(k): v
		}
		kind: _kind
	}
}
