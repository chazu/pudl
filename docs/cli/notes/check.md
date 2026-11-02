**A check observes and reports; it never changes anything.** For each named
model it populates the catalog, compares `desired` with what was observed,
evaluates checks and records the result. Apply changes with the tool that owns
them, then check again to verify.

**Exactly the named models.** `pudl check a b c` runs `a`, `b` and `c` and
nothing else. A model runs after every named model it lists in `depends_on`;
dependencies that are not named add no ordering and are never run. When a
named producer does not succeed, its dependents are recorded `blocked` and do
not run; independent models continue. The order and the edges are recorded in
the report (`ordered_members`, `edges`, `plan_digest`).

**One report.** A check records one report whether it names one model or
several: an aggregate `summary` (execution, conformity, checks, verification),
and one entry per model under `members` with its result, run ID, summary and
full `report` (populate, drift, checks). Read it again with
[`pudl report`](#pudl-report). Under `--json` the check writes that report as
its only output. Human output shows each model's findings as it concludes, then
the summary; internal catalog keys such as `//models/<name>` appear only in the
JSON evidence.

**Catalog replay is not an observation.** `--from-catalog` set-diffs `desired`
against records already in the catalog, so its verdict describes what was
*recorded*, not what is live. A replay therefore never writes a model status;
the model keeps the verdict of its last real observation. The scope is
mandatory because there is no way to infer which ingested records belong to a
model, and a replay names exactly one model. A model with `desired` state
populates its own snapshot and compares against it without `--from-catalog`.

**Interrupting a check.** The first Ctrl-C (SIGINT or SIGTERM) stops the check
cleanly: the running command receives SIGTERM and has 10 seconds to finish
(then it is killed), temporary workspaces are removed, the model in progress
concludes `cancelled`, the remaining models are recorded `cancelled`, and the
check's status is `cancelled`. A second Ctrl-C exits immediately (status 130),
still removing temporary workspaces. Check again to continue. A populate
`timeout` expiry is recorded as `failed`, not `cancelled`.

**Exit status.** By default `pudl check` exits 0 whenever every model
completed, including when it found drift; the report says what it found. A
failing fail-severity check or any other error exits nonzero.

With `--detailed-exitcode` the exit status is the result, so a script or CI job
can gate on it without parsing the report:

| Exit | Meaning |
|------|---------|
| `0` | Clean: no drift, every fail-severity check passed or is waived |
| `2` | Findings: drift (including a `--from-catalog` replay's) or a failing fail-severity check (with `--fail-on new`, one with a new unwaived finding) — and nothing else went wrong |
| `1` | Error: the check could not establish the answer (bad flags, unknown model, populate failure) |

Under this flag every error exits 1, so `2` always means findings. When several
models fail, the check exits 2 only when every failed model failed on its
fail-severity checks alone; models blocked or cancelled behind them do not
count as separate errors.

**Waivers.** A model's `waivers` list declares accepted, time-boxed
exceptions to its checks' findings, reviewed and versioned with the checks.
Each names an `owner`, a `reason`, a `created` date and an `expires` date
(`YYYY-MM-DD`, all required), and one target: a check's witnesses, or one
compliance-policy violation.

```cue
waivers: [{
	check:   "nat-ip-not-in-use"   // an expect: "empty" check of this model
	witness: {address: "https://.../addresses/nat-ip-2"}  // values the witness must carry
	owner: "network-team", reason: "held for failover"
	created: "2026-11-01", expires: "2027-01-31"
}, {
	policy: "gcp.#FirewallBestPractice", rule: "gcp-firewall-world-ingress"
	resource: "<resource ID from pudl policy show>"
	owner: "security", reason: "public bastion until the VPN lands"
	created: "2026-11-01", expires: "2026-12-31"
}]
```

A policy waiver applies to any check of the model whose witnesses carry
`resource` and `policy` arguments (such as `policy_noncompliant`): a witness is
waived when every violation it stands for — narrowed by any `rule`, `severity`,
`path` or `kind` argument it also carries — has a waiver. Loading fails on a
missing field, an invalid date, `expires` before `created`, or a check the model
does not have.

A waived finding stays in the report with its waiver (`witnesses[].waiver`).
A check whose every finding is waived has outcome `waived` (not a pass; the
summary counts it in `waived_checks`) and does not fail the check or
`--detailed-exitcode`. A waiver applies through its `expires` date (UTC); after
that its findings count again and the report flags it as `expired`. A waiver
that matches no finding is reported `stale`. The model report's `waivers` list
gives each waiver's state and how many findings it matched. `pudl policy show`
and `pudl show` do not mark waived violations: waivers belong to a model's
checks.
**Changes over time.** Each check's failing matches are compared with the
model's earlier reports of the same check over the same scope (same mode,
query, expectation, check scope and observation scope). A check result
records `findings` — every failing match by a stable `key`, with a `state`
and `first_seen` time — and `changes`, the counts by state and the report
compared with (`since`):

| State | Meaning |
|-------|---------|
| `new` | Failing now; no comparable report since the last complete one recorded it |
| `persisting` | Failing now and recorded before; `first_seen` is the earliest record |
| `resolved` | Recorded before, not failing now, and this observation is complete |
| `unknown` | Recorded before and not seen now, but this observation cannot establish its absence |

The key is a digest of the check relation's head arguments without evidence
identifiers (`entry_id`, `evidence_id`, `observation_id`, `snapshot_id`,
`run_id`), so a re-observed resource keeps its key and row order never
matters. Project identifying fields in the head: a
`policy_noncompliant(resource, policy, severity)` violation is one finding per
resource and policy. Only a complete observation resolves: a partial
snapshot, a run-scoped check over an incomplete observation, a check that
reached no verdict (for example because an input failed), or more failing
matches than the 1000 recorded leaves earlier findings `unknown`. Earlier
reports without a verdict are skipped; incomplete ones still count as having
seen what they recorded. The baseline is the stored reports, not earlier
snapshots evaluated again: they are the findings pudl recorded under the rules
in force then, and they survive snapshot retention, so changing a rule can
show as resolved and new findings. A waived finding keeps its state and is
marked `waived` (`changes.waived` counts them); waiving never resolves a
finding, and once its waiver expires it counts again. The summary counts
`new_findings` and `resolved_findings`. Compare two named reports with
[`pudl report ID --since ID2`](#pudl-report).

**Failing only on new findings.** `--fail-on new` makes fail-severity check
failures fail the command only when one of them has a `new` finding that no
active waiver excuses (the
first report of a check has nothing to compare with, so its failures are
new). A check whose only failures are known exits 0 and says so on stderr.
With `--detailed-exitcode` it exits 0 instead of 2; drift still exits 2, and
errors, checks without a verdict, and blocked or cancelled models still exit
1 or 2 as above. The report still records every failure.

**Command populate.** A `#CommandObserve` arm runs plain commands that print
JSON (an array of records, or a stream of objects) — `gcloud`, `kubectl`,
`aws … --format=json` — directly, with no shell. Fan out with a CUE
comprehension and stamp fields the tool omits with `set`:

```cue
populate: {
	schema: "pudl/gcp.#Firewall"     // optional; routes records like import --schema
	runs: [for p in ["prod-a", "prod-b"] {
		argv: ["gcloud", "compute", "firewall-rules", "list", "--project=\(p)", "--format=json"]
		set: project: p
	}]
}
```

The environment is inherited (cloud CLI credentials work). Any failing run
fails the populate phase and nothing is ingested; the error quotes the end of
its stderr. argv and `set` are recorded with the model: keep secrets out.
`pudl check --populate 'command:<cmdline>'` checks one ad hoc.

**Checks-only models.** A model without `populate` evaluates its checks over
the catalog as it is — e.g. data brought in with `pudl import` — after syncing
projected facts ([projection](projection.md)). It cannot declare `desired`.

**Retired commands, fields and flags.** `pudl check` replaces `pudl run` and
`pudl run set`. `--converge` and its flags, `--max-observation-age`, and the
model fields `converge`, `plugins`, `sealed_inputs`, `sealed_outputs`, `inputs`
and `bindings`, and the `#PluginObserve` and `#EweTarget` populate arms were
removed. A model that uses one fails to load with an error naming the field; see
[retired commands](retired-commands.md).
