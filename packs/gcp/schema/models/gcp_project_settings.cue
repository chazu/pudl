package models

import (
	"list"
	"regexp"
)

// Collectors of project-wide settings (schema/gcp/project.cue): enabled
// services and Binary Authorization policies. Both search CAI and fall back
// to gcloud per project.
//
// Live parity (2026-11-05, a sample of projects, field names only):
//   - services: CAI's copy holds name, parent and state; gcloud adds config
//     (the service's documentation, quotas and so on) and names the service
//     projects/N/services/S. gcloud also lists
//     cloudsecuritycompliance.googleapis.com, which CAI omits, so it is not
//     collected.
//   - Binary Authorization: identical except that gcloud adds etag and
//     updateTime and names the project by ID where CAI names it by number.
//     gcloud also prints the default policy of a project that never set
//     one; CAI omits those, and so does binauthz-policy.sh.

// CAI names a Binary Authorization policy's project by number only, so the
// type is searched per project (whatever _gcpAssetScope says), and the run
// names the project by ID.
_gcpAssetByProject: "binauthz-policies": true

_gcpAssetTypes: {
	services:            "serviceusage.googleapis.com/Service"
	"binauthz-policies": "binaryauthorization.googleapis.com/Policy"
}

_gcpSchemas: {
	services:            "gcp.#EnabledService"
	"binauthz-policies": "gcp.#BinaryAuthorizationPolicy"
}

_gcpCommands: services: ["services", "list", "--enabled"]

// Helper: the project's policy, unless it was never set.
_gcpRuns: "binauthz-policies": [{
	argv: ["sh", "../../populators/gcp/binauthz-policy.sh", "{{item}}"]
	set: project: "{{item}}"
}]

_gcpShapes: {
	services: {
		in: {...}
		let r = in
		out: {
			for k, v in r if k != "config" && k != "name" {(k): v}
			name: regexp.ReplaceAll("^.*/", r.name, "")
		}
	}
	// The project is the record's project field; name repeats it.
	"binauthz-policies": {
		in: {...}
		let r = in
		out: {for k, v in r if !list.Contains(["name", "etag", "updateTime"], k) {(k): v}}
	}
}

// Services CAI does not export (see above).
_gcpServicesCAIOmits: "^(.*/)?cloudsecuritycompliance\\.googleapis\\.com$"

_gcpSkip: services: {
	in: {...}
	let r = in
	skip: regexp.Match(_gcpServicesCAIOmits, r.name)
}
