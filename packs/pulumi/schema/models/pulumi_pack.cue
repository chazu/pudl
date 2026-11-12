package models

import (
	"list"
	"regexp"
	"strings"

	sm "pudl.schemas/pudl/systemmodel@v0"
)

// Pulumi content pack models (README: packs/pulumi/README.md in the PUDL
// source).
//
//   pulumi-stacks             reads every stack of _pulumiOrg and its latest
//                             update from Pulumi Cloud with GET requests,
//                             after the token guard (pulumi_guard.cue) passes;
//   pulumi-resources          reads the resources of every stack's state, the
//                             same way;
//   pulumi-fixture-stacks,
//   pulumi-fixture-resources  read the bundled fixtures in
//                             .pudl/populators/pulumi.

// ---- Live settings. The first two have no default: pulumi-stacks refuses
// to run until you set them in another file of package models, e.g.
// .pudl/schema/models/pulumi_settings.cue:
//
//	package models
//
//	_pulumiOrg:       "my-org"
//	_pulumiTokenName: "pudl-reader"

// The Pulumi Cloud organization.
_pulumiOrg: =~"^[A-Za-z0-9][A-Za-z0-9._-]*$"

// The name of the org access token pudl accepts in PUDL_PULUMI_TOKEN. Its
// role must reach only read scopes (pulumi_guard.cue).
_pulumiTokenName: string & !=""

// The Pulumi Cloud API origin; change it only for a self-hosted Pulumi Cloud.
// pudl sends the token there and nowhere else.
_pulumiAPI: *"https://api.pulumi.com" | =~"^https://[^/?#@]+$"

// ---- End of live settings. ----

// Secret envelopes of Pulumi state. No kept record may hold one.
_pulumiRejectKeys: ["4dabf18193072939515e22adb298388d", "ciphertext"]

// _pulumiStackTransform keeps the fields of pulumi.#Stack from one update:
// the vcs.* and git.head, git.headName and git.dirty environment keys, and
// pulumi.version. Everything else of the update is dropped.
_pulumiStackTransform: {
	in: _
	let env = *in.environment | {}
	out: {
		version:   in.version
		kind:      in.kind
		result:    in.result
		startTime: in.startTime
		if (in.endTime & int) != _|_ {endTime: in.endTime}
		if (in.resourceCount & int) != _|_ {resourceCount: in.resourceCount}
		if (env."pulumi.version" & string) != _|_ {pulumiVersion: env."pulumi.version"}
		source: {
			if (env."vcs.owner" & string) != _|_ && (env."vcs.repo" & string) != _|_ {
				repo: "\(env."vcs.owner")/\(env."vcs.repo")"
			}
			if (env."vcs.kind" & string) != _|_ {vcs: env."vcs.kind"}
			if (env."vcs.root" & string) != _|_ {root: env."vcs.root"}
			if (env."git.head" & string) != _|_ {commit: env."git.head"}
			if (env."git.headName" & string) != _|_ {branch: env."git.headName"}
			dirty: (*env."git.dirty" | "") == "true"
		}
	}
}

// Every stack of the organization, as <org>/<project>/<stack>.
_pulumiStacksEach: {
	get:  "/api/user/stacks?organization=\(_pulumiOrg)"
	list: "stacks"
	item: {in: _, out: "\(in.orgName)/\(in.projectName)/\(in.stackName)"}
	page: {next: "continuationToken", param: "continuationToken"}
}

#PulumiStacks: sm.#SystemModel & {
	name: "pulumi-stacks"
	// Per organization, so reading another one marks nothing of this one gone.
	observation: {scope: "pulumi/\(_pulumiOrg)/stacks", complete: true}
	populate: sm.#HTTPObserve & {
		schema: "pulumi.#Stack"
		base:   _pulumiAPI
		auth: {env: "PUDL_PULUMI_TOKEN", scheme: "token"}
		guard: _pulumiGuard
		each:  _pulumiStacksEach
		runs: [{
			get:       "/api/stacks/{{item}}/updates?pageSize=1&page=1"
			records:   "updates"
			set: stack: "{{item}}"
			transform: _pulumiStackTransform
		}]
		reject_keys: _pulumiRejectKeys
	}
}

// The fixture stacks: each file is the updates array of one stack's
// /updates response. scratch has no update, so no record.
_pulumiFixtureStacks: {
	dev:     "example-org/example-project/dev"
	prod:    "example-org/example-network/prod"
	scratch: "example-org/example-network/scratch"
}

#PulumiFixtureStacks: sm.#SystemModel & {
	name: "pulumi-fixture-stacks"
	observation: {scope: "fixture/pulumi/stacks", complete: true}
	populate: sm.#CommandObserve & {
		schema: "pulumi.#Stack"
		runs: [for file, id in _pulumiFixtureStacks {
			argv: ["cat", "../../populators/pulumi/updates-\(file).json"]
			set: stack: id
			transform: _pulumiStackTransform
		}]
	}
}

// _pulumiGCPNamed lists the GCP types whose Pulumi id is the relative
// resource name of the real resource (projects/P/...), which the GCP pack's
// schemas declare as their gcp.resource key. Both spellings of the service
// account type occur across provider versions.
_pulumiGCPNamed: [
	"gcp:compute/network:Network",
	"gcp:compute/subnetwork:Subnetwork",
	"gcp:compute/address:Address",
	"gcp:compute/globalAddress:GlobalAddress",
	"gcp:compute/forwardingRule:ForwardingRule",
	"gcp:compute/globalForwardingRule:GlobalForwardingRule",
	"gcp:compute/router:Router",
	"gcp:serviceaccount/account:Account",
	"gcp:serviceAccount/account:Account",
]

// _pulumiGCPName builds live.name, the gcp.resource key, from a GCP
// resource's id. Types not listed yield no name, so they never link.
//
//   - Listed types: the id as is.
//   - GKE clusters: Pulumi names every location locations/L, but a zonal
//     cluster's selfLink (the GCP pack's key) says zones/Z. A zone is a
//     region with a letter suffix.
//   - Buckets: the id is the bare bucket name; the key is b/<name>, the
//     Storage API's relative name.
_pulumiGCPName: {
	type: string
	id:   string
	out: [
		if list.Contains(_pulumiGCPNamed, type) {id},
		if type == "gcp:container/cluster:Cluster" {
			regexp.ReplaceAll("^(projects/[^/]+/)locations/([a-z]+-[a-z]+[0-9]+-[a-z])/", id, "${1}zones/${2}/")
		},
		if type == "gcp:storage/bucket:Bucket" {"b/\(id)"},
		_|_,
	][0]
}

// _pulumiResourceTransform keeps the fields of pulumi.#Resource from one
// state resource. It drops inputs, every output except the identity values
// in live, propertyDependencies, additionalSecretOutputs and initErrors.
// live keeps a value only when it is a plain string, so a secret envelope at
// an identity path is dropped too.
_pulumiResourceTransform: {
	in: _
	let outputs = *in.outputs | {}
	let meta = *outputs.metadata | {}
	let gcpName = (_pulumiGCPName & {type: in.type, id: *in.id | ""}).out
	out: {
		urn:      in.urn
		type:     in.type
		custom:   *in.custom | false
		external: *in.external | false
		if (in.id & string) != _|_ {id: in.id}
		if (in.protect & bool) != _|_ {protect: in.protect}
		if (in.parent & string) != _|_ {parent: in.parent}
		if (in.dependencies & [...string]) != _|_ {dependencies: in.dependencies}
		if (in.created & string) != _|_ {created: in.created}
		if (in.modified & string) != _|_ {modified: in.modified}
		if (in.sourcePosition & string) != _|_ {sourcePosition: in.sourcePosition}
		if (in.provider & string) != _|_ {
			// <provider URN>::<provider ID>: keep the URN.
			let parts = strings.Split(in.provider, "::")
			if len(parts) > 1 {provider: strings.Join(list.Slice(parts, 0, len(parts)-1), "::")}
		}
		if strings.HasPrefix(in.type, "kubernetes:") {
			live: metadata: {
				if (meta.uid & string) != _|_ {uid: meta.uid}
				if (meta.name & string) != _|_ {name: meta.name}
				if (meta.namespace & string) != _|_ {namespace: meta.namespace}
			}
		}
		if (in.id & string) != _|_ && (gcpName & string) != _|_ {
			live: name: gcpName
		}
	}
}

#PulumiResources: sm.#SystemModel & {
	name: "pulumi-resources"
	observation: {scope: "pulumi/\(_pulumiOrg)/resources", complete: true}
	populate: sm.#HTTPObserve & {
		schema: "pulumi.#Resource"
		base:   _pulumiAPI
		auth: {env: "PUDL_PULUMI_TOKEN", scheme: "token"}
		guard: _pulumiGuard
		each:  _pulumiStacksEach
		runs: [{
			get: "/api/stacks/{{item}}/export"
			// A stack with no resources exports no resources array.
			records:          "deployment.resources"
			records_optional: true
			set: stack: "{{item}}"
			transform: _pulumiResourceTransform
		}]
		reject_keys: _pulumiRejectKeys
	}
}

// The fixture states: each file is the deployment.resources array of one
// stack's export. dev is hostile: it plants values that must not be kept.
_pulumiFixtureStates: {
	dev:  "example-org/example-project/dev"
	prod: "example-org/example-network/prod"
}

#PulumiFixtureResources: sm.#SystemModel & {
	name: "pulumi-fixture-resources"
	observation: {scope: "fixture/pulumi/resources", complete: true}
	populate: sm.#CommandObserve & {
		schema: "pulumi.#Resource"
		runs: [for file, id in _pulumiFixtureStates {
			argv: ["cat", "../../populators/pulumi/resources-\(file).json"]
			set: stack: id
			transform: _pulumiResourceTransform
		}]
	}
}
