# Getting started

This tutorial teaches pudl by doing. Part 1 observes a small Git inventory,
compares it with what you expect and follows a drift finding back to its
evidence. Part 2 governs a GCP inventory with policies, checks, owners and a
waiver. Both parts run offline from data bundled in pudl: you need no source
checkout, scripts or cloud credentials. Each part takes about ten minutes.

[Concepts](concepts.md) defines the terms used here.

## 1. Install pudl

With Go 1.26.10 or newer installed, install the current development version:

```bash
go install github.com/chazu/pudl@main
```

Put Go's binary directory on your `PATH` if it is not already there. The
default is `~/go/bin`, and a configured `GOBIN` overrides it. Then confirm that
pudl runs:

```bash
pudl version
```

You also need Git.

## Part 1: observe, catalog and find drift

### 2. Create a workspace and install the example

<!-- walkthrough:repository -->
```bash
git init pudl-tutorial
cd pudl-tutorial
```

<!-- walkthrough:setup -->
```bash
pudl init
pudl example install git-inventory
pudl model show git-inventory
pudl schema show pudl/git.#GitRepository
pudl model validate git-inventory
```

`pudl init` creates the workspace in `.pudl/`. Everything this tutorial
records stays there. The example installs a **model**, `git-inventory`, and
two inventories of a fictional repository, `local/demo`. They do not inspect
your own repository. Reinstalling is safe: identical files are left alone, and
files you edited are kept and reported as conflicts.

A model says how to observe a system and what to expect of it.
`git-inventory` observes by running `cat` on
`.pudl/populators/git-inventory/current.json`. A real model runs a Git or
forge CLI instead; any command that prints JSON records works. Its **desired
state** is one resource: `local/demo` with default branch `main`.

`model show` lists that resource with its **type schema**,
`pudl/git.#GitRepository`, and its resource type, `git.repository`. A type
schema is a CUE definition of what a resource is. `schema show` prints the file
that contains it. Its `_pudl` block declares `identity_fields: ["name"]`: the
repository name identifies the resource, so pudl can follow it across
observations.

### 3. Observe and check the baseline

<!-- walkthrough:baseline -->
```bash
pudl check git-inventory
```

`pudl check` runs the model's collector and stores what it printed as an
**observation snapshot** (`snap_...`). It classifies each record as
`pudl/git.#GitRepository`, catalogs it, and compares it with the desired
state. The observed default branch is `main`, as expected, so the report is
clean.

The last lines summarize the **check**: its ID (`check_...`) and, for each
model, its result and run ID. Note the check ID; step 6 reopens this report.

### 4. Change the inventory and find drift

<!-- walkthrough:change -->
```bash
cp .pudl/populators/git-inventory/changed.json .pudl/populators/git-inventory/current.json
pudl check git-inventory
```

The changed inventory has default branch `release`. The model still expects
`main`, so the report shows **drift**:

```text
- ~ git.repository/local/demo (changed): default_branch: release → want main
  - default_branch: expected "main", observed "release", previous "main" (snapshot snap_...)
  - observed_at: 2026-10-05T19:16:47Z
```

A drift finding names the resource and a reason. The reason is `changed` (a
desired field differs), `missing` (nothing observed has that identity),
`unidentifiable` (the desired record has no identity) or `ambiguous` (several
different records share the identity). Each differing field shows the
expected, observed and previous values.

pudl reports the difference and changes nothing. The baseline snapshot is
kept, so the earlier evidence is still there.

### 5. Read the report

<!-- walkthrough:repeat -->
```bash
pudl check git-inventory
pudl report
pudl report --json
```

The drift appears again under a new check ID. `pudl report` prints the newest
report, and `--json` prints the same report as a versioned JSON document. Its
`summary.conformity` is `drifted`. The model's own report is under
`members[0].report`, with the finding in `drift.drifted`. `ok: true` means the
check ran, not that the system is clean. For scripts, read
`summary.conformity` or use `--detailed-exitcode` (exit 2 on drift).

### 6. Follow the evidence

List the changed record. Then replace `RECORD_ID` below with the ID that the
list command prints, and `BASELINE_CHECK_ID` with the check ID from step 3.

<!-- walkthrough:evidence -->
```bash
pudl list --where default_branch=release --items-only
pudl show RECORD_ID --raw
pudl report BASELINE_CHECK_ID
```

Each record is a **catalog entry**. Its ID comes from its content, so the same
record always gets the same ID. The raw record shows
`default_branch: "release"` and its type. The baseline report is still clean:
reports are stored, not recomputed. `pudl show RECORD_ID --history` lists
every stored version and snapshot of that resource.

The [automated walkthrough test](TESTING.md#git-inventory-walkthrough) runs
the commands in Part 1 with only pudl, Git, `cat` and `cp` on `PATH`.

## Part 2: policies, checks and waivers

Continue in the same workspace. Install the GCP content pack. It contains draft
GCP type schemas, best-practice **policies**, collectors and **check
families**, plus gcloud-shaped fixtures:

```bash
pudl pack install gcp
pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers \
  gcp-fixture-network-hygiene-checks
```

A check names its models exactly. The three collectors run first because the
family lists them in `depends_on`. The family has no `populate`: it is
**checks-only** and reads the collectors' newest snapshots.

### 7. Policies

Every observation evaluates the policies that apply to each resource:

```bash
pudl policy list
pudl policy show gcp.#FirewallBestPractice
pudl query policy_noncompliant
```

A policy is a CUE schema that describes a good instance of a type. Here
`#FirewallBestPractice` says that no enabled ingress rule allows `0.0.0.0/0`.
Each resource is `compliant`, `noncompliant` or `unknown` (a field the rule
needs is missing), and each violation names its rule and severity. The results
are stored as facts, so `pudl query` reads them like any other Datalog
relation. The types are drafts, so results are marked provisional until
someone sets `review: status: "validated"` on the type.

### 8. Checks and findings

The family's checks are Datalog queries that are expected to return nothing.
Each row they return is a **finding**. `policy-high` (severity `fail`) and
`nat-ip-not-in-use` fail, so the command exits 1. `policy-any` has severity
`warn`, so it is reported but does not fail the command. Run the same check
again:

```bash
pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers \
  gcp-fixture-network-hygiene-checks --fail-on new --detailed-exitcode
```

Each finding is now `persisting`. A finding that disappears from a complete
observation becomes `resolved`. If the observation is incomplete, it is
`unknown` instead, because missing evidence never resolves a finding. With
`--fail-on new` only new failures fail the command, so this run exits 0.

### 9. Owners

Findings name the owner of each resource. A type's `_pudl.owner` sets it from
labels or from a table of projects. Map the fixture project to a team by
creating `.pudl/schema/gcp/owners_local.cue`:

```cue
package gcp

_gcpProjectOwners: {"example-project": "platform"}
```

### 10. Waivers

Accept the open firewall until a date. Create
`.pudl/schema/models/waivers.cue`; its resource ID is the one that
`pudl policy show` printed:

```cue
package models

#GCPFixtureNetworkHygieneChecks: waivers: [{
	policy:   "gcp.#FirewallBestPractice"
	rule:     "gcp-firewall-world-ingress"
	resource: "d7bd91db579e44e82d630a47af35f25fc9b084aeb1c5715bee139b2aa9524744"
	owner:    "security"
	reason:   "Public bastion until the VPN lands"
	created:  "2026-11-01"
	expires:  "2030-12-31"
}]
```

Validate the model and check again, showing only the platform team's findings:

```bash
pudl model validate gcp-fixture-network-hygiene-checks
pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers \
  gcp-fixture-network-hygiene-checks --owner platform
```

Findings are grouped under `owner platform`, and `policy-high` is `waived`,
citing the waiver's owner, expiry and reason. A waived finding keeps its state.
It is not resolved. When the waiver expires, the finding counts again.
`nat-ip-not-in-use` still fails, so the command still exits 1.

## Next steps

- Collect from a real project with the [GCP pack](../packs/gcp/README.md)
  or the [AWS pack](../packs/aws/README.md).
- Write your own collector with
  `pudl model new NAME --populate 'command:...'`, or import files with
  `pudl import --path FILE`.
- Write types and policies with the [schema authoring guide](schema-authoring.md),
  and rules with the [Datalog guide](datalog.md).
- See [evidence and recovery](evidence.md) for retention, exports and
  portable bundles.
