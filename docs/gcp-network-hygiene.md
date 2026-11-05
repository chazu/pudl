# GCP network hygiene walkthrough

Find an enabled ingress ALLOW rule with source `0.0.0.0/0`, inspect its
retained evidence, and verify a correction. The fixture walkthrough needs only
pudl and `cp`; it makes no cloud requests. For the full GCP inventory, see the
[GCP content pack](../packs/gcp/README.md).

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

The model's own report is `members[0].report`. Note the check ID
(`run_set_id`) and the model report's `populate.snapshot_id`; below they are
`BASELINE_CHECK` and `BASELINE_SNAPSHOT`.

## Inspect data without writing a rule

```bash
pudl policy show 'pudl/gcphygiene.#FirewallNoInternetIngress'
pudl list --collection-id BASELINE_SNAPSHOT --where name=public-ssh --select name --select 'sourceRanges[*]' --all --json
pudl show RECORD_ID --field 'sourceRanges[*]'
```

`policy show` lists the rule and the failing resource. Substitute the listed
entry's `id` for `RECORD_ID`; plain `pudl show RECORD_ID` prints the whole
record and its compliance section, with the violation at `sourceRanges[*]`.

`--where` matches typed values, nested paths and wildcard membership; repeat
it to AND conditions. `--select` adds fields to each returned entry. See
[`pudl list`](cli-reference.md#pudl-list).

## Save a check and verify the correction

Save a second check over the latest snapshot of the population:

```bash
pudl model new firewall-audit --check policy_noncompliant --evidence scope:fixture/example-project/firewalls --max-age 15m
cp .pudl/populators/gcp-network-hygiene/fixed.json .pudl/populators/gcp-network-hygiene/current.json
pudl check gcp-network-hygiene --detailed-exitcode --json
pudl check firewall-audit --detailed-exitcode --json
```

Both checks now pass. The first observes the corrected fixture, and its
finding is reported `resolved`. The second evaluates the latest retained
snapshot of the scope and labels its evidence as recorded.

Note the new `members[0].report.populate.snapshot_id` as `FIXED_SNAPSHOT`:

```bash
pudl snapshot show FIXED_SNAPSHOT --compare BASELINE_SNAPSHOT --json
pudl show RECORD_ID --history --json
pudl report BASELINE_CHECK --json
```

The comparison shows the source-range change. History lists the resource's
stored versions and the snapshots that observed them. The original report
still holds the original failing finding.

## Collect real projects

The installed `schema/models/gcp_network_hygiene.cue` also contains the separate
`gcp-network-hygiene-live` model. It lists the projects your normal `gcloud`
credentials can see (`gcloud projects list`) when it runs, so ensure they can
list projects and firewall rules, then run:

```bash
pudl model validate gcp-network-hygiene-live
pudl check gcp-network-hygiene-live --detailed-exitcode --json
```

This model runs `gcloud compute firewall-rules list` once per listed project
and injects that project into each record; the report names the projects under
`populate.inputs`. It declares a complete observation, so do not add a limit
or a partial filter to the command. A project whose command fails is a failed
input: the others are ingested and the observation is incomplete.

The policy identifies broad IPv4 ALLOW declarations. Effective reachability also
depends on priorities, DENY rules, targets and other policy layers; this example
does not calculate packet reachability or cover IPv6 and hierarchical policies.
Adapt the policy to the rule you intend to check; `pudl policy evaluate
--what-if FILE` tries an edited copy without storing results. The `gcp`
content pack ships a fuller firewall policy.
