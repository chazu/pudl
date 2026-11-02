---
name: pudl-core
description: Using the pudl CLI to discover, catalog and govern cloud infrastructure — collectors and imports, CUE type schemas, compliance policies, Datalog checks, findings, waivers and reports. Use when working with pudl commands, .pudl/ workspaces, schemas, policies, models or check reports.
---

# PUDL Core

pudl observes infrastructure and reports on it; it never changes it. It runs
collectors (plain `gcloud`/`aws`/`kubectl ... --format json` commands) or
imports files, stores each observation as a snapshot in a local SQLite catalog
(`.pudl/` per repository; `~/.pudl` with `--global`), classifies every record
against a CUE type schema, evaluates compliance policies per resource and runs
Datalog checks. Hand fixes to the tool that owns the resource.

The binary is the source of truth: `pudl prime` (briefing), `pudl guide
<topic>` (overview, observe, schemas, policies, checks, query, doctor, packs,
agents), `pudl help [CMD] --json` (commands and flags).

## Agent workflow

```bash
pudl init                                   # idempotent; repairs .pudl/
pudl example install gcp                    # or aws; or write a collector
pudl model list --json
pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers \
  gcp-fixture-network-hygiene-checks --json --detailed-exitcode
pudl report --json                          # findings, owners, change states
# fix with the owning tool, or add a waiver to the model; then check again
pudl report <new-check-id> --since <old-check-id> --json
```

- `pudl check MODEL...` runs exactly the named models, producers first by
  `depends_on`, and records one report. Name a check family together with the
  collectors it reads; models that were not named never run.
- Outcomes: pass, fail, unknown, error, waived. Missing, incomplete or stale
  evidence is unknown, never pass.
- Findings carry an `owner` and a change state (new, persisting, resolved,
  unknown). Only a complete observation resolves a finding.
- Scaffold, then edit the printed path: `pudl model new`, `pudl rule new`,
  `pudl schema new --from <entry-id> --path <pkg>:#<Def>`.
- Preview writes: `import --dry-run`, `doctor --fix --dry-run`,
  `policy evaluate --dry-run` or `--what-if FILE`, `snapshot prune --dry-run`.

## JSON contracts and exit codes

- `check --json` / `report [ID] --json`: `report_version` 3; `run_set_id` is
  the check ID; `summary` (execution, conformity, checks, verification,
  new_findings); `members[].report` per model. `report ID --since ID2`
  classifies findings per check.
- `doctor --json`: `report_version` 3 with `repairs[]`. `policy
  list|show|evaluate --json`: `report_version` 1.
- `list --json` is paginated (`total_matched`; `--all` for every row).
  `show ID --json` includes `compliance`. `import --json` is one element per file.
- `help --json` marks hidden commands `"advanced": true`.
- Exit: 0 ok; 1 error or failed check. With `--detailed-exitcode`, 2 means
  drift or a failing fail-severity check. `--fail-on new` fails only on
  findings no comparable earlier report recorded. warn and info checks never
  fail the command.

## Collectors (models)

```cue
package models

import sm "pudl.schemas/pudl/systemmodel@v0"

#Firewalls: sm.#SystemModel & {
	name: "firewalls"
	observation: {scope: "gcp/firewalls", complete: true}
	populate: {
		schema: "gcp.#Firewall"                 // route records like import --schema
		runs: [for p in ["prod-a", "prod-b"] {
			argv: ["gcloud", "compute", "firewall-rules", "list", "--project=\(p)", "--format=json"]
			set: project: p                     // add a field the output omits
		}]
	}
}
```

- argv runs without a shell, with your environment. `populate.each: {argv,
  select?}` enumerates inputs at run time; runs use `"{{item}}"`.
- A failed run is a failed input: other runs are ingested, the snapshot is
  incomplete (`populate.failed_inputs` in the report), checks over it are
  unknown, and the model fails. `fail_fast: true` restores all-or-nothing.
- A model without `populate` is checks-only. `desired: [...]` reports
  inventory drift; nothing is changed.
- One-off: `pudl check --populate 'command:<cmdline>'` (writes no model).
- Snapshots: `pudl snapshot list|show|current|retain|prune`;
  `snapshot show NEW --compare OLD`; `pudl show ID --history`.

## Checks and waivers

```cue
checks: [{name: "policy-high", query: "policy_noncompliant", expect: "empty",
	severity: "fail", message: "Fix violations", evidence: ["model:firewalls"], max_age: "1h"}]
waivers: [{policy: "gcp.#FirewallBestPractice", rule: "gcp-firewall-world-ingress",
	resource: "<resource ID>", owner: "security", reason: "bastion until VPN",
	created: "2026-11-01", expires: "2026-12-31"}]
```

- `evidence`: snapshot IDs, `current`, `model:<name>` or `scope:<name>`.
- A waiver targets `check` + `witness: {arg: value}`, or `policy` + `rule` +
  `resource`. `owner`, `reason`, `created`, `expires` (YYYY-MM-DD) are
  required. Waived findings stay in the report as `waived`; expired waivers
  stop applying; unmatched waivers are reported stale.
- `pudl model new NAME --check RELATION --evidence model:X --max-age 1h` saves
  a relation as a checks-only model. Evidence must cover every relation the
  query reads, or the check is unknown. `--owner TEAM` filters printed findings.

## Type schemas

```cue
#Firewall: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.compute.firewall"
		identity_fields: ["selfLink"]          // resource ID; quote dotted keys
		sensitive_fields: ["metadata.items[*].value"]   // "[REDACTED]" before storage
		facts: gcp_firewall: args: {name: "name", disabled: {path: "disabled", default: false}}
		owner: {paths: ["labels.team"], map: {path: "project", owners: _owners}, fallback: "platform"}
		review: {status: "validated", by: "sre-platform", at: "2026-11-01"}   // default draft
	}
	...
}
```

- Records are not queryable until `facts` projects them. Each fact carries
  `entry_id` and `resource_id`; join one record's relations on `entry_id`.
  `{exists: path}` and `default` stand in for negation.
- Ownership projects `resource_owner` / `resource_unowned`.
- Policy results against a draft type are provisional.
- `secret_scan_ignore: [paths]` silences doctor's secret warning for a path.
- `@pudl(match)` on a body field (`kind: "compute#firewall" @pudl(match)`)
  claims records for the type: a claimed record that fails the type is
  rejected, not stored as a generic type.
- Explicit `--schema` is strict; `--allow-schema-fallback` permits fallback.

## Compliance policies

A policy is a standalone definition: `_pudl: {schema_type: "policy",
base_schema: "<pkg>.#Type"}`. Never `#Policy: #Type & {...}`; it does not
load. Results are `policy_status` / `policy_violation` facts; checks use
`policy_noncompliant(resource, policy, severity)`.

- `field: value @pudl(rule="name", severity="low|medium|high|critical", doc="...")`.
- An absent required field makes the result unknown unless
  `@pudl(absent="violation")` is on it or an ancestor, or the type's review
  status is validated.
- `&&` and `||` do not short-circuit over absent fields (the policy silently
  passes); use `list.Contains([if x != _|_ {x}], v)`.
- Annotate fields outside comprehensions (`f?: _ @pudl(rule=...)`) and
  constrain inside; otherwise the policy fails to load.
- A field required only inside a comprehension is never reported absent;
  anchor the rule as a conflict on a field that is present.
- A policy cannot constrain a sensitive field.
- `pudl policy evaluate --what-if draft.cue` tries a draft; `pudl policy
  list|show` reads results; `pudl show ID` has a compliance section.

See `docs/schema-authoring.md#compliance-policies`.

## Query and rules

```bash
pudl query --list
pudl query gcp_address status=RESERVED --json   # typed: false, 22, '"22"'
pudl query -f draft.cue my_relation              # try a rule file
pudl rule add rules.cue
pudl model deps && pudl query impacted_by changed=<model>
```

Rules are CUE: `name: {head: {rel, args}, body: [{rel, args}, ...]}` with
`$`-prefixed variables. `catalog_entry` is join-only. `pudl query` warns on
stderr about rules naming relations or args nothing produces, and about stale
projections; read those warnings when a check passes unexpectedly. Writing
facts by hand (`facts add/retract/invalidate`) is advanced; prefer projection.

## Doctor

`pudl doctor` diagnoses; `pudl doctor --fix [--only NAME] [--dry-run]`
repairs: schema-names, policy-leaves, inference, identity, projections,
policy-results, data-permissions, and sensitive-scrub (opt-in, only when
named; rotate any credential it redacts).

## Do not use (retired)

`pudl run`, `run set`, `run report` (use `pudl check` / `pudl report`);
`converge`, approvals, sealed values, value bindings and model `inputs`;
`pudl mu ...`, the `#PluginObserve` and `#EweTarget` populate arms;
`schema reinfer`, `schema migrate`, `reclassify`, `migrate identity`,
`facts reproject` (use `pudl doctor --fix`).
Models with retired fields fail to load naming the field. See
`docs/retired-commands.md`.
