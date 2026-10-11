# GCP network hygiene walkthrough

Find an enabled ingress ALLOW rule with source `0.0.0.0/0`, inspect its retained
evidence, and verify a correction. The fixture journey requires PUDL and standard
`cat`/`cp` utilities; it makes no cloud requests and needs no Mu installation.

## Install and check the fixture

In a directory for this workspace:

```bash
pudl init
pudl example install gcp-network-hygiene
pudl run gcp-network-hygiene --detailed-exitcode --json
```

The last command intentionally exits 2. The report identifies `public-ssh`,
protocol `tcp`, port `22`, and the source record ID. Its check selects only the
new complete snapshot. The fixture also contains a DENY rule with a public
source and a private-source ALLOW rule; neither is this finding.

Retain the report's `run_id`, `populate.snapshot_id`, and the finding's
`evidence_ids[0]`. Below, substitute them for `BASELINE_RUN`,
`BASELINE_SNAPSHOT`, and `RECORD_ID`.

## Inspect data without writing a rule

```bash
pudl list --collection-id BASELINE_SNAPSHOT --where name=public-ssh --select name --select 'sourceRanges[*]' --all --json
pudl show RECORD_ID --field 'sourceRanges[*]'
```

`--where` supports typed equality, nested paths, and wildcard membership. Repeat
it for AND conditions. `--select` adds chosen fields to each returned entry.
Missing fields are distinguished from explicit null values. Filtering happens
before pagination; `--all` enumerates every match, subject to an explicit limit.
Payload scans fail visibly if a file is unreadable or the inspection budget is
exceeded; they never silently return a partial answer.

## Save a check and verify the correction

The installed example includes the relationship rule. Save a second check over
the latest snapshot of its population:

```bash
pudl model new firewall-audit --check gcp_internet_ingress_allow --evidence scope:fixture/example-project/firewalls --max-age 15m
cp .pudl/populators/gcp-network-hygiene/fixed.json .pudl/populators/gcp-network-hygiene/current.json
pudl run gcp-network-hygiene --detailed-exitcode --json
pudl run firewall-audit --detailed-exitcode --json
```

Both runs now pass. The first observes the corrected fixture; the second checks
the selected retained snapshot and labels its evidence as recorded. Neither
claims to have inspected a real GCP project. This is a data-quality check of an
explicit fixture source.

Retain the new `populate.snapshot_id` as `FIXED_SNAPSHOT`:

```bash
pudl snapshot show FIXED_SNAPSHOT --compare BASELINE_SNAPSHOT --json
pudl show RECORD_ID --history --json
pudl run report BASELINE_RUN --json
```

The comparison shows the source-range change. History lists distinct stored
versions and their observation snapshots, including re-observations of identical
content. The original report retains the original failing finding.

`pudl doctor --json` includes a reviewable `repairs` list when inference,
projection, or unresolved schema references need attention. Each item gives
preview/inspection and apply arguments. Doctor does not apply those changes.

## Collect a real project

The installed `schema/models/gcp_network_hygiene.cue` also contains the separate
`gcp-network-hygiene-live` model. Set its `_project` to your project ID and ensure
your normal `gcloud` credentials can list firewall rules, then run:

```bash
pudl model validate gcp-network-hygiene-live
pudl run gcp-network-hygiene-live --detailed-exitcode --json
```

This model uses `gcloud compute firewall-rules list` with an explicit project
and injects that project into each record. Its completeness declaration assumes
the collector retrieves the entire list; do not add a limit or partial-location
filter while retaining that declaration. Failed commands publish no snapshot.

The check identifies broad IPv4 ALLOW declarations. Effective reachability also
depends on priorities, DENY rules, targets and other policy layers; this example
does not calculate packet reachability or cover IPv6 and hierarchical policies.
Adapt the schema/rules to the precise policy you intend to check.

The fixture workflow is covered by `TestGCPNetworkHygieneWorkflow` in `cmd`.
Live GCP access and a human usability trial are separate acceptance activities.
