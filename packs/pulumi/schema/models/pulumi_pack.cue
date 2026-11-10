package models

import sm "pudl.schemas/pudl/systemmodel@v0"

// Pulumi content pack models (README: packs/pulumi/README.md in the PUDL
// source).
//
//   pulumi-stacks          reads every stack of _pulumiOrg and its latest
//                          update from Pulumi Cloud with GET requests, after
//                          the token guard (pulumi_guard.cue) passes;
//   pulumi-fixture-stacks  reads the bundled fixtures in .pudl/populators/pulumi.

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

#PulumiStacks: sm.#SystemModel & {
	name: "pulumi-stacks"
	// Per organization, so reading another one marks nothing of this one gone.
	observation: {scope: "pulumi/\(_pulumiOrg)/stacks", complete: true}
	populate: sm.#HTTPObserve & {
		schema: "pulumi.#Stack"
		base:   _pulumiAPI
		auth: {env: "PUDL_PULUMI_TOKEN", scheme: "token"}
		guard: _pulumiGuard
		each: {
			get:  "/api/user/stacks?organization=\(_pulumiOrg)"
			list: "stacks"
			item: {in: _, out: "\(in.orgName)/\(in.projectName)/\(in.stackName)"}
			page: {next: "continuationToken", param: "continuationToken"}
		}
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
