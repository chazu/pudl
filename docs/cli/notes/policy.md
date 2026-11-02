Policies are written in CUE ([Compliance Policies](schema-authoring.md#compliance-policies)).
pudl evaluates them after every import and observation; these commands
inspect the stored results and re-evaluate them. Results are facts
(`policy_status`, `policy_violation`), so `pudl query` and checks read them;
the built-in rule `policy_noncompliant` is the usual check query (see
[governance](governance.md#use-policies-in-checks)). A result is stale when
the policy or its type changed since it was evaluated; `pudl policy evaluate`
or `pudl doctor --fix --only policy-results` refreshes it.
