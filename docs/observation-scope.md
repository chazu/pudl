# Observation scope and completeness

Checks normally read latest-known facts across the active catalog. That view
combines observations from different times; a resource missing from one later
partial import remains known. It does not establish a complete current inventory.

For inventory checks, declare what a collector covers and select its snapshots:

```cue
observation: {
    scope: "gcp/prod/firewalls"
    complete: true
}
populate: {
    schema: "pudl/gcp.#Firewall"
    runs: [{argv: ["gcloud", "compute", "firewall-rules", "list",
        "--project=prod", "--format=json"], set: project: "prod"}]
}
checks: [{
    name: "no-open-ingress"
    query: "open_ingress"
    expect: "empty"
    severity: "fail"
    message: "Review internet-accessible firewalls"
    evidence: ["current"]
    max_age: "15m"
}]
```

`complete` is an explicit collector assertion. The collector must successfully
finish every required page and fan-out member before asserting it. A successful
exit or a JSON array alone does not imply completeness. Command failure publishes
no replacement observation; reported plugin errors prevent complete status.
Omitted completeness defaults to false, including for older stored snapshots.
An explicitly permitted schema mismatch also makes the observation incomplete.
Command observers can opt into exploratory fallback with
`populate.allow_schema_fallback: true`; their report records the policy and
mismatch count.

`populate.schema` identifies the population's schema even when it has zero
records. For other observers, `observation.schemas: ["package.#Definition"]`
can declare coverage explicitly. Declaring coverage does not fix invalid schema
definitions or incomplete collection.

Evidence selectors are exact snapshot IDs, `current` (this run's observation or
selected replay snapshot), `model:<name>` (the latest recorded observation), or
`scope:<name>` (the latest observation of an explicitly named population).
Scope selection rejects multiple source owners; choose exact snapshot IDs to
resolve that ambiguity. Multiple selectors enable cross-source checks and retain
each source's time independently. Differing versions of one resource are
ambiguous rather than silently resolved by selector order.

Selected checks rebuild projected facts from only those snapshots in a private,
temporary SQLite catalog. They use the active schemas and rules; stored reports
retain their original findings. They do not modify the source's latest-known
facts or mix unrelated generic assertions into the selected inventory. Payload
materialization is bounded to 256 MiB; exceeding the bound yields an unknown
check rather than a partial answer. The temporary catalog is discarded afterward.

A partial, missing, stale, ambiguous, or unreadable selection produces an unknown
check with structured diagnostics. `max_age` requires a positive duration and
explicit evidence selectors. No age bound means no freshness requirement, not
an assertion that the selected records are live. Reports retain snapshot IDs,
scope, source, observation time, age, and completeness; their existing retention
policy pins referenced snapshots.

After a complete observation of A and B, a complete observation containing only
B establishes A's absence within that second snapshot. Checks selecting the
second snapshot see B alone. The first snapshot and A's historical facts remain
available. A partial observation cannot establish that absence.

Inventory identity now uses the same nested and quoted field-path extraction as
import. Missing declared identity fields make a record unidentifiable; the
name/path/id fallback applies only when no fields are declared. Composite match
keys preserve component boundaries. Persisted resource IDs are unchanged.

`snapshot show NEW --compare OLD` compares populations with the same scope and
owner. A disappearance is labeled `removed` only when the new snapshot is
complete; otherwise it is `not-observed`. Inventory drift likewise reports
unobserved desired resources as uncertain for partial inventories, exits with an
error, and cannot promote them to verified status. Existing matching resources
can still satisfy ensure-present expectations in a partial inventory.

For saved observation envelopes, `pudl mu ingest-observe --scope NAME --complete`
records an explicit completeness assertion. `--coverage-schema` declares a
covered schema even for an empty population. Stored replay remains distinct
from a new live observation.
