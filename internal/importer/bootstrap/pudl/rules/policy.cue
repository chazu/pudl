package rules

// Compliance over the policy result facts that policy evaluation writes at
// every import and observation, and `pudl policy evaluate` refreshes (see
// docs/design/2026-11-01-compliance-policies.md):
//   policy_status    : resource, policy, type, status, policy_digest
//   policy_violation : resource, policy, rule, severity, path, kind, declared_by
//
// policy_noncompliant(resource, policy, severity) — a noncompliant result,
// once per severity among its violations. Constrain severity to select a
// threshold, e.g. a #Check with `query: "policy_noncompliant"` and
// `expect: "empty"`, or `pudl query policy_noncompliant severity=critical`.
policy_noncompliant: {
	head: {rel: "policy_noncompliant", args: {resource: "$R", policy: "$P", severity: "$S"}}
	body: [
		{rel: "policy_status", args: {resource: "$R", policy: "$P", status: "noncompliant"}},
		{rel: "policy_violation", args: {resource: "$R", policy: "$P", severity: "$S"}},
	]
}
