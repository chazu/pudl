package models

import (
	"list"
	"strings"
)

// The token guard of the live Pulumi collectors (README: packs/pulumi/README.md
// in the PUDL source). Before any other request, pudl reads the token's
// identity, its assigned role, and the org's policies and permission sets.
// The check passes only for the org access token _pulumiTokenName of
// _pulumiOrg whose one role reaches only scopes in _pulumiReadScopes.

_pulumiGuard: {
	get: {
		user:     "/api/user"
		roles:    "/api/orgs/\(_pulumiOrg)/roles?uxPurpose=role&assignedToMe=true"
		policies: "/api/orgs/\(_pulumiOrg)/roles?uxPurpose=policy"
		sets:     "/api/orgs/\(_pulumiOrg)/roles?uxPurpose=set"
	}
	check: {
		in: _
		// A personal token has no tokenInfo; a team token names its team.
		let token = *in.user.tokenInfo | {}
		_token: (*token.organization | "") == _pulumiOrg &&
			(*token.name | "") == _pulumiTokenName &&
			(*token.team | null) == null
		let roles = *in.roles.roles | []
		_oneRole: len(roles) == 1
		let ids = {for it in list.Concat([*in.policies.roles | [], *in.sets.roles | []]) {(it.id): it.details}}
		// The role's tree, then the trees of the policies and sets it names,
		// then those they name. Sets name nothing, so the last round must
		// name nothing either.
		_r0: _#PulumiTree & {roots: [if _oneRole {roles[0].details}]}
		_r1: _#PulumiTree & {roots: [for id in _r0.ids if ids[id] != _|_ {ids[id]}]}
		_r2: _#PulumiTree & {roots: [for id in _r1.ids if ids[id] != _|_ {ids[id]}]}
		_r3: _#PulumiTree & {roots: [for id in _r2.ids if ids[id] != _|_ {ids[id]}]}
		_walk: {
			let rounds = [_r0, _r1, _r2, _r3]
			ok: _oneRole && len(_r3.ids) == 0 &&
				!list.Contains([for r in rounds {r.ok}], false) &&
				len([for r in rounds for id in r.ids if ids[id] == _|_ {id}]) == 0
			scopes: list.Concat([for r in rounds {r.scopes}])
		}
		_extra: [for s in _walk.scopes if _pulumiReadScope[s] == _|_ {s}]
		ok:     _token && _oneRole && _walk.ok && len(_extra) == 0
		reason: [
			if !_token {"PUDL_PULUMI_TOKEN must be org access token \(_pulumiTokenName) of \(_pulumiOrg); a personal or team token is refused"},
			if !_oneRole {"PUDL_PULUMI_TOKEN must have exactly one role"},
			if !_walk.ok {"the role of PUDL_PULUMI_TOKEN has a descriptor pudl does not know, or refers to a policy or set the token cannot list"},
			"the role of PUDL_PULUMI_TOKEN grants scopes outside pudl's read allow list: \(strings.Join([for s in _extra if s =~ "^[a-z_]+:[a-z_]+$" {s}], ", "))",
		][0]
	}
}

// _#PulumiNode is one node of a role's descriptor tree: a descriptor, or the
// ID of a policy or set. known is false for an unknown __type. A condition
// only narrows where its sub-node applies, so its sub-node counts in full.
_#PulumiNode: {
	n:     _
	_isID: (n & string) != _|_
	type: [if !_isID {*n."__type" | ""}, ""][0]
	known: _isID || type == "PermissionDescriptorAllow" || type == "PermissionDescriptorGroup" ||
		type == "PermissionDescriptorCompose" || type == "PermissionDescriptorCondition"
	children: [
		if type == "PermissionDescriptorGroup" {n.entries},
		if type == "PermissionDescriptorCompose" {n.permissionDescriptors},
		if type == "PermissionDescriptorCondition" {[n.subNode]},
		[],
	][0]
	scopes: [if type == "PermissionDescriptorAllow" {n.permissions}, []][0]
	ids: [if _isID {[n]}, []][0]
}

// _#PulumiTree reads descriptor trees four levels deep (a group of
// conditions on composes of IDs needs four). ok is false for an unknown
// node or a deeper tree. ids and scopes are those the trees name. It is not
// recursive: CUE 0.16 reports a cycle in a recursive walk.
_#PulumiTree: {
	roots: [...]
	_l0: [for r in roots {_#PulumiNode & {n: r}}]
	_l1: [for p in _l0 for c in p.children {_#PulumiNode & {n: c}}]
	_l2: [for p in _l1 for c in p.children {_#PulumiNode & {n: c}}]
	_l3: [for p in _l2 for c in p.children {_#PulumiNode & {n: c}}]
	let all = list.Concat([_l0, _l1, _l2, _l3])
	ok: !list.Contains([for x in all {x.known}], false) &&
		len([for p in _l3 for c in p.children {c}]) == 0
	ids: list.Concat([for x in all {x.ids}])
	scopes: list.Concat([for x in all {x.scopes}])
}

// The scopes a token's role may reach: Pulumi Cloud's read, list and view
// scopes, from its scope reference (stacks, environments, cloud accounts,
// organization settings) and from read-only custom roles seen in practice.
// stack:export is the one export; no decrypt, encrypt, open, write, create,
// update or delete scope is here. A role that reaches any other scope is
// refused, so a new Pulumi read scope needs an entry here first.
_pulumiReadScopes: [
	// Stacks
	"stack:read", "stack:export", "stack:list_deleted",
	"stack_access:read", "stack_annotations:read",
	"stack_deployment:read", "stack_deployment_cache:read", "stack_deployment_settings:read",
	"stack_schedule:read", "stack_webhook:read",
	"project_annotations:read", "organization_annotations:read",
	// Environments (environment:open decrypts and is not here)
	"environment:read", "environment:rotate_history", "environment:list_deleted",
	"environment_access:read", "environment_schedule:read", "environment_settings:read",
	"environment_tag:read", "environment_tags:list", "environment_version:read",
	"environment_webhook:read",
	// Cloud accounts
	"insights_account:read", "insights_account_access:read", "insights_account_scan:read",
	// Organization
	"agent_pool:read", "audit_logs:read", "ai_conversations:list_all", "ai_conversations:read",
	"auth_policies:read", "deployments:read", "deployments:read_usage",
	"integrations:read", "invites:read", "oidc_issuers:read", "org_integrations:read",
	"org_member:read", "org_requests:read", "org_token:read", "organization:read_usage",
	"organization_webhook:read", "policy_exceptions:read", "policy_groups:read",
	"policy_pack:read", "policy_results:read", "resources:dashboard", "resources:search",
	"role:read", "saml:read", "scim:read", "services:read", "tags:read",
	"team:list", "team:list_tokens", "team:read", "templates:read", "templates_source:read",
]

_pulumiReadScope: {for s in _pulumiReadScopes {(s): true}}
