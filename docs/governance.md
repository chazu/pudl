# Governance: checks, findings and waivers

This guide covers the governance loop: declare what must hold, check it
against what pudl observed, attribute each failure to an owner, accept known
exceptions until a date, track what is new, and gate CI on the result. pudl
only reads and reports. Fix a finding with the tool that owns the resource,
then check again.

The examples use the GCP content pack's fixtures, so you can run them offline:

```bash
pudl init
pudl pack install gcp
pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers \
  gcp-fixture-network-hygiene-checks --detailed-exitcode
```

## Write a check

A check is a Datalog query on a model, with an expectation and a severity:

```cue
checks: [{
	name:     "policy-high"
	query:    "gcp_policy_noncompliant_high"  // any relation: a rule head or a fact relation
	expect:   "empty"                          // or "nonempty"
	severity: "fail"                           // "info", "warn" or "fail"
	message:  "Resolve high and critical policy violations"
	evidence: ["model:gcp-firewalls", "model:gcp-addresses"]  // optional
	max_age:  "1h"                             // optional; needs evidence
}]
```

- **Without `evidence`**, the check reads latest-known facts across the
  catalog. **With `evidence`**, it reads only the selected snapshots:
  `current` (this model's observation), `model:<name>` (that model's latest
  observation), `scope:<name>` or an exact snapshot ID. See
  [observation scope](observation-scope.md#select-evidence).
- **Only `fail` severity** affects the exit status. `warn` and `info`
  failures are reported but do not fail the check.
- **A checks-only model** has no `populate`. It evaluates its checks over what
  the catalog already holds. List its collectors in `depends_on`, and name
  them together on the command line so they run first:

```cue
#NetworkChecks: sm.#SystemModel & {
	name:       "network-checks"
	depends_on: ["gcp-firewalls", "gcp-addresses"]
	checks: [...]
}
```

`pudl model new NAME --check RELATION --evidence model:COLLECTOR --max-age 1h`
scaffolds a checks-only model around an existing relation.

## Read the result

Each check reaches one outcome:

| Outcome | Meaning |
|---------|---------|
| `pass` | The expectation held |
| `fail` | The expectation did not hold; the failing rows are findings |
| `waived` | It failed, but an active waiver covers every finding |
| `unknown` | The evidence could not establish an answer: missing, stale, incomplete or ambiguous evidence, a failed input, or a relation nothing produces |
| `error` | Evaluation failed, for example an invalid rule |

`unknown` and `error` are never passes, whatever the severity. Each carries
structured `diagnostics` in the report.

A failing `expect: "empty"` check reports its rows:

- **witnesses**: up to 20 rows with their `arguments`, `owner` and any
  `waiver`. `count` is the total.
- **findings**: every failing row (up to 1000), each with a stable `key`, a
  change `state`, `first_seen`, `resource` and `owner`, whether or not it is
  a witness or waived. Only witnesses also record their `arguments`.

A failing `expect: "nonempty"` check has one finding with no arguments.

`pudl check` records one report per invocation, however many models it names.
Read it again with `pudl report [ID]`. In `--json`, the report is
`report_version` 3; `run_set_id` is the check ID; `summary` aggregates every
model; and each model's full report (populate, drift, checks, waivers) is
under `members[].report`.

## Use policies in checks

Policies judge one resource at a time (see
[Compliance Policies](schema-authoring.md#compliance-policies)). pudl stores
each result as facts (`policy_status`, `policy_violation`). The built-in rule
`policy_noncompliant(resource, policy, severity)` yields one row per
noncompliant resource, policy and violated severity. A check over it fails on
any violation within its evidence:

```cue
{name: "policy-any", query: "policy_noncompliant", expect: "empty",
 severity: "warn", message: "Review every policy violation"}
```

For a threshold, write a rule that keeps the `severity` argument in its head,
so a policy waiver can tell which violations a finding stands for:

```cue
noncompliant_high: {
	head: {rel: "noncompliant_high", args: {resource: "$R", policy: "$P", severity: "high"}}
	body: [{rel: "policy_noncompliant", args: {resource: "$R", policy: "$P", severity: "high"}}]
}
noncompliant_critical: {
	head: {rel: "noncompliant_high", args: {resource: "$R", policy: "$P", severity: "critical"}}
	body: [{rel: "policy_noncompliant", args: {resource: "$R", policy: "$P", severity: "critical"}}]
}
```

To inspect results outside a check, use `pudl policy show POLICY` (failing
resources, most severe first), `pudl show ID` (each policy's result for one
entry) and `pudl query policy_noncompliant severity=high`.

## Waive a finding

A waiver accepts a finding until a date. Waivers live on the model next to
its checks, so they are reviewed and versioned with them. Every waiver needs
`owner`, `reason`, `created` and `expires` (`YYYY-MM-DD`), and one target:

```cue
waivers: [{
	// One compliance-policy violation: policy, rule and resource are all required.
	policy:   "gcp.#FirewallBestPractice"
	rule:     "gcp-firewall-world-ingress"
	resource: "<resource ID from pudl policy show>"
	owner: "security", reason: "public bastion until the VPN lands"
	created: "2026-11-01", expires: "2026-12-31"
}, {
	// Witnesses of one expect: "empty" check of this model.
	check:   "nat-ip-not-in-use"
	witness: {address: "https://.../addresses/nat-ip-2"}  // values the witness must carry
	owner: "network", reason: "held for failover"
	created: "2026-11-01", expires: "2027-01-31"
}]
```

- **A check waiver** covers a witness that carries every value in `witness`.
- **A policy waiver** applies to any check of the model whose witnesses carry
  `resource` and `policy` arguments, such as `policy_noncompliant`. A witness
  is waived when every violation it stands for has a waiver. Any `rule`,
  `severity`, `path` or `kind` argument on the witness narrows the violations
  it stands for.
- **A waived finding** stays in the report with its waiver. A check whose
  every finding is waived has outcome `waived`; it does not fail the command
  or the exit status, and the summary counts it in `waived_checks`.
- **Expiry.** A waiver applies through its `expires` date (UTC). After that
  the finding counts again and the report marks the waiver `expired`. A
  waiver that matches nothing is `stale`. The model report's `waivers` list
  gives each waiver's `state` and `matches`.
- **Loading fails** on a missing field, an invalid date, `expires` before
  `created`, a check the model does not have, or a check that expects
  `nonempty`.

Waivers belong to a model's checks. `pudl policy show` and `pudl show` still
report a waived violation.

## Track changes over time

Each check compares its findings with the model's earlier reports of the
same check over the same scope (same query, expectation, check scope and
observation scope). Each finding gets a state:

| State | Meaning |
|-------|---------|
| `new` | Failing now; not recorded since the last complete report |
| `persisting` | Failing now and recorded before; `first_seen` is the earliest record |
| `resolved` | Recorded before, not failing now, and this observation is complete |
| `unknown` | Recorded before, not seen now, and this observation cannot establish its absence |

- **Keys.** A finding's `key` is a digest of the check head's arguments
  without evidence identifiers (`entry_id`, `evidence_id`, `observation_id`,
  `snapshot_id`, `run_id`). A re-observed resource keeps its key, so put
  identifying fields in the rule head.
- **Only complete evidence resolves.** A partial snapshot, a failed input, a
  check without a verdict, or more than 1000 failing rows leaves earlier
  findings `unknown`.
- **The baseline is the stored reports**, not old snapshots evaluated again.
  If you change a rule, its findings can show as resolved and new.
- **Waivers do not resolve.** A waived finding keeps its state and is counted
  in `changes.waived`.

Each check's `changes` gives the counts and the report it compared with
(`since`). The summary counts `new_findings` and `resolved_findings`.

Compare two stored reports, model by model:

```bash
pudl report check_B --since check_A
```

## Attribute findings to owners

A type schema's `_pudl.owner` names each resource's team (see
[owner](schema-authoring.md#owner)). pudl attributes a finding to the owner of
the resource its arguments name: a resource ID (`resource_id`, `resource`,
`target` or `name`), or else a catalog entry ID (`entry_id`, `evidence_id`,
`observation_id`). A finding that names neither, such as one keyed by a
selfLink, has no owner (`""` in JSON). A resolved finding keeps the owner its
earlier report recorded.

```bash
pudl check ... --owner payments                 # print only payments' findings
pudl policy show gcp.#BucketBestPractice --owner payments
pudl query resource_unowned                     # resources nobody owns
```

`--owner` filters what is printed; the recorded report, its counts and the
exit status stay complete. Human output groups each check's findings by
owner. To report unowned resources, add a check:

```cue
{name: "unowned", query: "resource_unowned", expect: "empty", severity: "warn",
 message: "Label these resources with a team or map their project"}
```

## Trust incomplete evidence correctly

A collector that fans out runs one command per project, account or region.
When one run fails, pudl:

- ingests the records of the runs that succeeded;
- marks the snapshot incomplete, so nothing it covered counts as removed;
- lists the failure in the report under `populate.failed_inputs` (argv, exit
  status, stderr tail, and the `each` item when there is one);
- makes the checks over that observation `unknown`
  (`incomplete_observation` or `failed_inputs`), and fails the model.

A check over a collector that failed is therefore never a pass, and its
earlier findings become `unknown`, not `resolved`. A family named with a
failed collector is recorded `blocked`. If every run fails, nothing is
ingested. `populate.fail_fast: true` makes any failure fail the whole
populate. A failure whose stderr matches `populate.empty_when` (an API
disabled in the project, say) is not a failed input: it is an empty input,
observed with no records, listed under `populate.empty_inputs`, and leaves
the snapshot complete. See [observation scope](observation-scope.md) for
scope, completeness, empty inputs and evidence selection.

## Run in CI

```bash
pudl check collector-a collector-b family-checks --detailed-exitcode --fail-on new --json > report.json
```

With `--detailed-exitcode`, the exit status is the result:

| Exit | Meaning |
|------|---------|
| `0` | Clean: no drift, and every fail-severity check passed or was waived |
| `2` | Findings, and nothing else went wrong: drift, or a failing fail-severity check |
| `1` | Error: the check could not establish the answer (bad flags, unknown model, a failed populate, a check without a verdict) |

Every error exits 1, so `2` always means findings. When several models fail,
the command exits 2 only when each failed on its fail-severity checks alone.
Without `--detailed-exitcode`, a failing fail-severity check or any error
exits 1, and drift alone exits 0.

`--fail-on new` lets known failures pass: a fail-severity check fails the
command only when it has a `new` finding that no active waiver covers. On a
check's first report every finding is new. A run whose failures are all known
exits 0 and says so on stderr. Drift, errors and checks without a verdict
still exit 2 or 1. The report records every failure either way.

For machines:

- `--json` writes the report as the only stdout document. An early failure
  writes one error document instead, with `code`, `message`, `exit_code` and
  suggestions.
- `--progress-json` writes progress events (`type: "progress"`) to stderr.
- `summary.next_actions` names the command that reads the report again.
- Ctrl-C stops a check cleanly. The models still running and those not yet
  started are recorded `cancelled`. A second Ctrl-C exits at once with status
  130.
