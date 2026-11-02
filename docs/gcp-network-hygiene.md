# GCP network hygiene walkthrough

Find an enabled ingress ALLOW rule with source `0.0.0.0/0`, inspect its retained
evidence, and verify a correction. The fixture journey requires PUDL and standard
`cat`/`cp` utilities; it makes no cloud requests.

## Install and check the fixture

In a directory for this workspace:

```bash
pudl init
pudl example install gcp-network-hygiene
pudl check gcp-network-hygiene --detailed-exitcode --json
```

The last command intentionally exits 2. The example's schema file holds the
`#Firewall` type and a policy, `#FirewallNoInternetIngress`, whose rule
`internet-ingress-allow` (high) rejects an enabled INGRESS rule with `allowed`
entries and source `0.0.0.0/0`. PUDL evaluates the policy as the snapshot is
ingested; the model's check queries the built-in `policy_noncompliant` rule
over the new complete snapshot. Its one finding is the `public-ssh` resource.
The fixture also contains a DENY rule with a public source and a
private-source ALLOW rule; both are compliant. See
[Compliance Policies](schema-authoring.md#compliance-policies).

The model's own report is `members[0].report`. Retain the check's
`run_set_id` and the model report's `populate.snapshot_id`. Below, substitute
them for `BASELINE_CHECK` and `BASELINE_SNAPSHOT`.

## Inspect data without writing a rule

```bash
pudl policy show 'pudl/gcphygiene.#FirewallNoInternetIngress'
pudl list --collection-id BASELINE_SNAPSHOT --where name=public-ssh --select name --select 'sourceRanges[*]' --all --json
pudl show RECORD_ID --field 'sourceRanges[*]'
```

`policy show` lists the rule and the failing resource. Substitute the listed
entry's `id` for `RECORD_ID`; plain `pudl show RECORD_ID` also prints its
compliance section, with the violation at `sourceRanges[*]`.

`--where` supports typed equality, nested paths, and wildcard membership. Repeat
it for AND conditions. `--select` adds chosen fields to each returned entry.
Missing fields are distinguished from explicit null values. Filtering happens
before pagination; `--all` enumerates every match, subject to an explicit limit.
Payload scans fail visibly if a file is unreadable or the inspection budget is
exceeded; they never silently return a partial answer.

## Save a check and verify the correction

Save a second check over the latest snapshot of the population:

```bash
pudl model new firewall-audit --check policy_noncompliant --evidence scope:fixture/example-project/firewalls --max-age 15m
cp .pudl/populators/gcp-network-hygiene/fixed.json .pudl/populators/gcp-network-hygiene/current.json
pudl check gcp-network-hygiene --detailed-exitcode --json
pudl check firewall-audit --detailed-exitcode --json
```

Both checks now pass. The first observes the corrected fixture; the second checks
the selected retained snapshot and labels its evidence as recorded. Neither
claims to have inspected a real GCP project. This is a data-quality check of an
explicit fixture source.

Retain the new `members[0].report.populate.snapshot_id` as `FIXED_SNAPSHOT`:

```bash
pudl snapshot show FIXED_SNAPSHOT --compare BASELINE_SNAPSHOT --json
pudl show RECORD_ID --history --json
pudl report BASELINE_CHECK --json
```

The comparison shows the source-range change. History lists distinct stored
versions and their observation snapshots, including re-observations of identical
content. The original report retains the original failing finding.

`pudl doctor --json` includes a `repairs` list diagnosing schema names,
unresolved schema references, inference drift, identities, projected facts and
policy results.
Each repair that is needed gives preview and apply arguments. Plain `doctor`
writes nothing; `pudl doctor --fix [--only NAME]` applies the repairs and
`--fix --dry-run` previews them.

## Collect a real project

The installed `schema/models/gcp_network_hygiene.cue` also contains the separate
`gcp-network-hygiene-live` model. Set its `_project` to your project ID and ensure
your normal `gcloud` credentials can list firewall rules, then run:

```bash
pudl model validate gcp-network-hygiene-live
pudl check gcp-network-hygiene-live --detailed-exitcode --json
```

This model uses `gcloud compute firewall-rules list` with an explicit project
and injects that project into each record. Its completeness declaration assumes
the collector retrieves the entire list; do not add a limit or partial-location
filter while retaining that declaration. Failed commands publish no snapshot.

The policy identifies broad IPv4 ALLOW declarations. Effective reachability also
depends on priorities, DENY rules, targets and other policy layers; this example
does not calculate packet reachability or cover IPv6 and hierarchical policies.
Adapt the policy to the rule you intend to check; `pudl policy evaluate
--what-if FILE` tries an edited copy without storing results. The `gcp`
content pack ships a fuller firewall policy.

The fixture workflow is covered by `TestGCPNetworkHygieneWorkflow` in `cmd`.
Live GCP access and a human usability trial are separate acceptance activities.
