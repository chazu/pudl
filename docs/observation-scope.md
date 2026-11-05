# Observation: collectors, scope and completeness

A model's `populate` runs commands that print JSON and ingests their output as
one **snapshot**. The snapshot records its **scope** (what population it
covers) and whether it is **complete**. Checks can then select exactly the
snapshots they trust, and pudl tells "absent" apart from "not observed".

## Write a collector

```cue
#ProdFirewalls: sm.#SystemModel & {
	name: "prod-firewalls"
	observation: {scope: "gcp/firewalls", complete: true}
	populate: {
		schema: "gcp.#Firewall"          // optional; routes records like import --schema
		runs: [for p in ["prod-a", "prod-b"] {
			argv: ["gcloud", "compute", "firewall-rules", "list", "--project=\(p)", "--format=json"]
			set: project: p              // stamp a field the tool leaves out
		}]
	}
}
```

| Field | Meaning |
|-------|---------|
| `runs[].argv` | One command, run directly with no shell. It prints a JSON array of records or a stream of JSON objects |
| `runs[].set` | Fields written into every record that run prints, overwriting them |
| `runs[].dir` | Working directory, relative to the model file |
| `runs[].transform` | `{in: _, out: ...}`: reshape each record before `set` and routing; `out: null` drops it (below) |
| `each` | A command whose JSON list supplies the fan-out inputs at run time (below) |
| `schema` | Route every record to this type; mismatches fail the populate |
| `allow_schema_fallback` | Permit exploratory fallback to a base or catchall schema; the observation is then incomplete |
| `timeout` | Per-run limit as a Go duration (default `10m`) |
| `fail_fast` | Fail the whole populate, ingesting nothing, on the first failed run |
| `empty_when` | Regular expressions (Go RE2) matched against a failed run's stderr; a match makes the run an empty input (below) |
| `fail_when` | Regular expressions (Go RE2) matched against the stderr of a run that exited 0; a match makes the run a failed input (below) |
| `fallback` | `{when, observe, partial?}`: another way to observe the same resources, used when these runs cannot answer (below) |

The environment is inherited, so cloud CLI credentials work. `argv` and `set`
are recorded with the model: keep secrets out of them. A record can route
itself with a quoted `"_schema"` field. `pudl check --populate
'command:<cmdline>'` tries one command without a model.

### Fall back to another source

A populate can name a second way to observe the same resources, used when its
own runs cannot answer: for example gcloud list commands behind a Cloud Asset
Inventory search.

```cue
populate: {
	schema: "gcp.#ComputeInstance"
	each: {argv: ["gcloud", "organizations", "list", "--format=json"], select: "name"}
	runs: [{argv: ["gcloud", "asset", "search-all-resources", "--scope={{item}}", ...]}]
	fallback: {
		when: ["SERVICE_DISABLED", "USER_PROJECT_DENIED", "PERMISSION_DENIED"]
		observe: {
			each: {argv: ["gcloud", "projects", "list", "--format=json"], select: "projectId"}
			runs: [{argv: ["gcloud", "compute", "instances", "list", "--project={{item}}", "--format=json"]
				set: project: "{{item}}"}]
			empty_when: ["SERVICE_DISABLED"]
		}
	}
}
```

The fallback is used instead of the populate's runs when:

- a run exits non-zero with stderr matching a `when` pattern (checked before
  `empty_when`; a timeout never matches);
- the `each` command fails that way; or
- `each` lists no inputs, e.g. credentials that see no organization.

The remaining runs are skipped and the records already read are discarded,
so the two sources are never mixed. The fallback has its own `runs`, `each`,
`timeout`, `fail_fast`, `empty_when` and `fail_when`. Its records are routed by the
populate's `schema`, and its failures are failed inputs as usual. It cannot
set a schema or declare a fallback of its own. The report says
`source: fallback` with what triggered it (JSON `populate.fallback`: `reason`
`failed_input`, `each_failed` or `no_inputs`, the matching `pattern` and
`stderr_line`, and the `trigger` run).

A fallback that sees only some of the resources declares `partial: true`. The
gcloud fallback of the GCP pack's IAM policy collector, for example, reads
each project's own policy, while the search it stands in for also returns
the policies of buckets, service accounts and secrets. A partial fallback's
observation is incomplete, so nothing it cannot see is marked gone; the
report says `partial fallback` (JSON `populate.fallback.partial`).

### Reshape records with a transform

When a tool prints records in a different shape from the schema, a run's
`transform` reshapes each one in CUE. `in` is the record as printed; `out` is
what is stored. `set` then applies to `out`, and the result is routed and
redacted as usual:

```cue
runs: [{
	argv: ["gcloud", "asset", "search-all-resources", "--scope=projects/{{item}}",
		"--asset-types=compute.googleapis.com/Instance",
		"--read-mask=name,assetType,versionedResources", "--format=json"]
	set: project: "{{item}}"
	transform: {
		in: _
		out: [if in.assetType != "compute.googleapis.com/Instance" {null},
			in.versionedResources[0].resource][0]
	}
}]
```

- `out: null` drops the record.
- Hidden helper fields are allowed inside the transform, e.g.
  `_r: in.versionedResources[0].resource`.
- An `out` that fails to evaluate (a field the record lacks, a conflict), is
  not concrete, or is not an object or null makes the run a failed input,
  like any other malformed output.
- The transform is not recorded with the model instance.
- Cost: about 0.2 ms per record of a few kilobytes.

### Enumerate inputs at run time

CUE comprehensions fix the fan-out when the file is written. To fan out over
whatever a command lists, give `each` a command that prints a JSON list of
strings, or of objects with `select` naming the field. Every run is repeated
per input with `{{item}}` replaced in its `argv` and `set`:

```cue
populate: {
	each: {argv: ["gcloud", "projects", "list", "--format=json"], select: "projectId"}
	runs: [{
		argv: ["gcloud", "compute", "firewall-rules", "list", "--project={{item}}", "--format=json"]
		set: project: "{{item}}"
	}]
}
```

With `each`, every run must use `{{item}}`; without it, none may. The report
records the inputs under `populate.inputs`. A failing `each` command fails the
populate with nothing ingested.

The report's `populate.records` counts the records observed in the snapshot,
the same number `pudl snapshot show` reports; `populate.new_records` counts
those stored as new catalog entries. Records are content-addressed, so a re-run
over unchanged resources reports every record under `records` and
`new_records: 0`.

### Failed inputs

A run that exits nonzero, times out or prints something other than JSON
objects is a **failed input**. The other runs' records are ingested, the
snapshot is incomplete, and the report lists each failure under
`populate.failed_inputs` (argv, exit status, stderr tail, and the `each` item).
The model fails, and its checks over that observation are `unknown`. If every
run fails, nothing is ingested. See
[governance](governance.md#trust-incomplete-evidence-correctly).

### Empty inputs

Some failures are a definite answer that an input has no resources. gcloud,
for example, exits 1 with `reason: SERVICE_DISABLED` on stderr when the
project's API is disabled, so the project has none of that type. List such
errors in `empty_when`:

```cue
populate: {
	each: {argv: ["gcloud", "projects", "list", "--format=json"], select: "projectId"}
	runs: [{argv: ["gcloud", "compute", "firewall-rules", "list", "--project={{item}}", "--format=json"], set: project: "{{item}}"}]
	empty_when: ["SERVICE_DISABLED"]
}
```

A run that exits nonzero with stderr matching one of the patterns is an
**empty input**: observed with zero records. It does not make the snapshot
incomplete or fail the model, so a model that depends on the collector is not
blocked. The report lists it under `populate.empty_inputs` (run, `each` item,
argv, exit status, the matched `pattern`, and `reason`, the stderr line it
matched), so it stays visible. A timeout, undecodable output, or a failure no
pattern matches (`PERMISSION_DENIED`, say) is still a failed input. A pattern
that is not a valid regular expression fails the model's load.

### Warnings that hide a failure

Some commands exit 0 after reading only part of what they were asked for.
gcloud's aggregated list commands (compute lists that span regions or
zones) do this when a project refuses the list: they print
`WARNING: Some requests did not succeed.` on stderr and an empty or partial
result on stdout. Taken at its word, the project would look empty and its
resources would be marked gone. List such warnings in `fail_when`:

```cue
populate: {
	each: {argv: ["gcloud", "projects", "list", "--format=json"], select: "projectId"}
	runs: [{argv: ["gcloud", "compute", "addresses", "list", "--project={{item}}", "--format=json"], set: project: "{{item}}"}]
	fail_when: ["Some requests did not succeed"]
}
```

A run that exits 0 with stderr matching one of the patterns is a **failed
input** with exit status 0: its records are discarded, the snapshot is
incomplete, and the report's error names the pattern and the stderr line it
matched. It is never an empty input, and it trips `fail_fast`. `fail_when`
does not apply to the `each` command.

## Declare scope and completeness

`observation.scope` names the population, for example `gcp/firewalls`.
`complete: true` asserts that the collector retrieved all of it: every page
and every fan-out member. A successful exit or a JSON array alone does not
imply completeness. Omitted, `complete` is false.

A snapshot is recorded incomplete, whatever the declaration, when:

- an input failed;
- `allow_schema_fallback` let a record fall back to another schema.

`populate.schema` identifies the population's type even when it has no
records; an empty JSON array is a valid zero-record snapshot. Other observers
can declare coverage with `observation.schemas: ["package.#Definition"]`.

**Absence needs completeness.** After a complete snapshot of A and B, a
complete snapshot holding only B shows that A is gone. A partial snapshot
cannot: A is `not-observed`, never `removed`, and its policy results stay
open. `pudl snapshot show NEW --compare OLD` uses the same rule for snapshots
of one scope and owner.

## Select evidence

Without `evidence`, a check reads **latest-known** facts across the catalog.
That view mixes observations from different times; a resource missing from a
later partial import stays known. It is not a current inventory.

A check with `evidence` reads only the snapshots its selectors name:

| Selector | Selects |
|----------|---------|
| `current` | This model's own observation from this check |
| `model:<name>` | That model's latest recorded observation |
| `scope:<name>` | The latest observation of that scope (an error if two sources own it) |
| a snapshot ID | Exactly that snapshot |

```cue
checks: [{
	name: "no-open-ingress", query: "open_ingress", expect: "empty", severity: "fail"
	message: "Review internet-accessible firewalls"
	evidence: ["current"]
	max_age:  "15m"
}]
```

- Several selectors combine sources, each keeping its own observation time.
  Two different versions of one resource across them are ambiguous, not
  resolved by order.
- `max_age` needs explicit selectors. Without it, there is no freshness
  requirement.
- The selected snapshots are rebuilt into a temporary catalog with the active
  schemas and rules, then discarded. Latest-known facts are not modified.
  Materialization is capped at 256 MiB; past it the check is `unknown`.
- A partial, missing, stale, ambiguous or unreadable selection makes the check
  `unknown`, with diagnostics.

Reports record each snapshot's ID, scope, source, observation time, age at
check and completeness. A report pins the snapshots it references against
pruning (see [evidence lifetime](evidence.md#reports-and-evidence-lifetime)).

## Desired state and replays

A model with `desired` compares it with its own snapshot and reports drift;
nothing is changed. In a partial snapshot, desired resources that were not
observed are uncertain, and the model cannot be recorded clean. Resources that
were observed can still satisfy their expectations.

`pudl check MODEL --from-catalog --catalog-scope SNAPSHOT_OR_ORIGIN` compares
`desired` with records already in the catalog instead of observing. A replay
describes what was recorded, not what is live, so it never updates the model's
status.
