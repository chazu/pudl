package models

import (
	"list"
	"regexp"
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
	"gke-clusters":     "container.googleapis.com/Cluster"
	"run-services":     "run.googleapis.com/Service"
	"pubsub-topics":    "pubsub.googleapis.com/Topic"
	"service-accounts": "iam.googleapis.com/ServiceAccount"
	"dns-zones":        "dns.googleapis.com/ManagedZone"
	"kms-keyrings":     "cloudkms.googleapis.com/KeyRing"
	"kms-keys":         "cloudkms.googleapis.com/CryptoKey"
	"iam-roles":            "iam.googleapis.com/Role"
	"service-account-keys": "iam.googleapis.com/ServiceAccountKey"
}

// CAI lists KMS key rings and keys in every location; the gcloud commands
// list only _gcpKMSLocations. The live search keeps only those locations, so
// a fallback run observes the same set and marks nothing as gone.
_gcpAssetInLocations: {
	"kms-keyrings": true
	"kms-keys":     true
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
		transform: _gcpAssetTransform & {
			_t: type
			if _gcpAssetInLocations[type] != _|_ {
				_locations: _gcpKMSLocations
			}
		}
	}]
}

// Unwraps a CAI search result into the type's shape. The project is the one
// in the resource's parent (the project itself, or e.g. a KMS key's ring).
// Dropped: other asset types, resources outside a project (an organization's
// custom roles), what _gcpSkip names, and with _locations resources in other
// locations.
_gcpAssetTransform: {
	_t: string
	_locations: *null | [...string]
	in: _
	_resource: in.versionedResources[0].resource
	_shaped:   (_gcpShapes[_t] & {in: _resource}).out
	_inProject: regexp.Match("^//[^/]+/projects/", in.parentFullResourceName)
	// CUE's && does not short-circuit; an unselected list element may fail.
	_elsewhere: [
		if _locations == null {false},
		!list.Contains(_locations, regexp.FindSubmatch("/locations/([^/]+)/", in.name)[1]),
	][0]
	_skip: [if _gcpSkip[_t] != _|_ {(_gcpSkip[_t] & {in: _resource}).skip}, false][0]
	// The first elements exist only for a record to drop, so they select
	// null; otherwise the shaped resource. (out must be declared outright:
	// a transform's in is unknown until a record arrives.)
	out: [
		if in.assetType != _gcpAssetTypes[_t] {null},
		if !_inProject {null},
		if _elsewhere {null},
		if _skip {null},
		_shaped & {project: regexp.FindSubmatch("projects/([^/]+)", in.parentFullResourceName)[1]},
	][0]
}

// Puts a gcloud list record into the type's shape, or drops what _gcpSkip
// names.
_gcpGcloudTransform: {
	_t:      string
	in:      _
	_record: in
	_skip: [if _gcpSkip[_t] != _|_ {(_gcpSkip[_t] & {in: _record}).skip}, false][0]
	out: [
		if _skip {null},
		(_gcpShapes[_t] & {in: _record}).out,
	][0]
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

	// gcloud adds a node pool's kubelet certificate expiry, which CAI lacks.
	"gke-clusters": {
		in: {...}
		let r = in
		out: {
			for k, v in r if k != "nodePools" {(k): v}
			if r.nodePools != _|_ {
				nodePools: [for np in r.nodePools {{for k, v in np if k != "kubeletCertInfo" {(k): v}}}]
			}
		}
	}
	"run-services":  _gcpSameShape
	"pubsub-topics": _gcpSameShape
	"kms-keyrings":  _gcpSameShape
	"kms-keys":      _gcpSameShape
	"iam-roles":     _gcpRoleShape
	// Key metadata is identical in both sources.
	"service-account-keys": _gcpSameShape

	// gcloud adds etag; CAI omits disabled when it is false (gcloud prints
	// it), so the shape always states it.
	"service-accounts": {
		in: {...}
		let r = in
		out: {
			for k, v in r if k != "etag" && k != "disabled" {(k): v}
			disabled: [if r.disabled != _|_ {r.disabled}, false][0]
		}
	}

	// CAI omits every kind field (on the zone and inside its configs) and
	// upper-cases visibility; the schema matches on the zone's kind and
	// expects lower-case visibility.
	"dns-zones": {
		in: {...}
		let r = in
		out: {
			for k, v in r if k != "kind" && k != "visibility" {
				(k): [if k == "labels" {v}, (_gcpNoKind2 & {in: v}).out][0]
			}
			kind: "dns#managedZone"
			if r.visibility != _|_ {
				visibility: strings.ToLower(r.visibility)
			}
		}
	}
}

// A record whose CAI copy is identical to gcloud's.
_gcpSameShape: {
	in: {...}
	let r = in
	out: r
}

// _gcpNoKindN removes kind fields N levels deep (a list's elements are one
// level below it), leaving other values as they are. CUE has no recursion,
// hence the levels.
_gcpNoKind2: {
	in: _
	let r = in
	out: [
		if (r & {...}) != _|_ {{for k, v in r if k != "kind" {(k): (_gcpNoKind1 & {in: v}).out}}},
		if (r & [...]) != _|_ {[for e in r {(_gcpNoKind1 & {in: e}).out}]},
		r,
	][0]
}

_gcpNoKind1: {
	in: _
	let r = in
	out: [
		if (r & {...}) != _|_ {{for k, v in r if k != "kind" {(k): v}}},
		if (r & [...]) != _|_ {[for e in r {(_gcpNoKind0 & {in: e}).out}]},
		r,
	][0]
}

_gcpNoKind0: {
	in: _
	let r = in
	out: [
		if (r & {...}) != _|_ {{for k, v in r if k != "kind" {(k): v}}},
		r,
	][0]
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
