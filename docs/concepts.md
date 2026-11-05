# Concepts

pudl answers three questions about your infrastructure: what exists, what it
looks like, and what is wrong with it. This page defines the terms the rest of
the documentation uses. [Getting started](getting-started.md) shows each one in
action.

pudl only reads. It never changes the systems it observes.

## Workspace

`pudl init` creates `.pudl/` in a repository. Schemas, models and rules under
`.pudl/schema/` are files you commit. The catalog, raw records, facts,
snapshots and reports under `.pudl/data/` are local state that stays out of
Git. Outside a repository workspace, or with `--global`, pudl uses `~/.pudl/`.
See [workspace](workspace.md).

## Observations and snapshots

An **observation** brings records into pudl in one of two ways:

- A **collector** runs commands. A model's `populate` field holds a
  `#CommandObserve`: one or more commands that print JSON records, such as
  `gcloud ... --format=json`, `aws ... --output json` or
  `kubectl get ... -o json`. pudl runs each command directly, with no shell.
  `pudl check` runs the collector.
- An **import** reads files: `pudl import --path FILE` (JSON, YAML, CSV or
  NDJSON, from a file, directory, pattern or stdin).

Each collector observation is stored as a **snapshot** (`snap_...`) with two
properties:

- **Scope** names the population the observation covers, for example
  `gcp/prod/firewalls` (`observation.scope` on the model).
- **Completeness** says whether the observation saw the whole scope
  (`observation.complete`, false when omitted). Only a complete snapshot can
  show that a resource is gone.

A collector can **fan out**: run one command per project, account or region.
List the runs in CUE, or use `each` to enumerate the inputs at run time from a
command such as `gcloud projects list`. If one run fails, it is a **failed
input**. pudl still ingests the other runs' records, but the snapshot is
incomplete and the report lists the failure. A failed input never makes
resources look deleted.

`pudl snapshot list|show|current` inspects snapshots, and
`pudl snapshot retain|prune` keeps or removes them. See
[observation scope](observation-scope.md) for evidence selection and
freshness.

### Collections

An NDJSON file is imported as a **collection**: one collection entry
(`pudl/core.#Collection`) plus one entry per line. Each item is classified and
identified on its own. Membership is stored separately, so one item can belong
to several collections without being stored twice. A JSON object is never
treated as a collection.

```bash
pudl list --collections-only
pudl list --collection-id COLLECTION_ID --schema aws/ec2.#Instance
pudl delete COLLECTION_ID --cascade --force
```

`--cascade` deletes the collection and its memberships. It deletes an item only
when no other collection still includes it.

## Entries and resources

Every stored record is a **catalog entry**. Its **entry ID** is the SHA-256
hash of its content, displayed as a pronounceable proquint such as
`vomap-foqab`. The same content always gets the same ID, so identical content
is stored once. Commands accept the proquint or the full hash.

A **resource** is the thing a record describes, such as a firewall rule or a
bucket, followed over time. Its **resource ID** is a hash of the type's family
and the values of the type's `identity_fields` (for example `selfLink`). When
a resource changes, it gets a new entry with the same resource ID, so pudl
keeps its **history**:

```bash
pudl show vomap-foqab --history
```

A type without identity fields falls back to the content hash, so every change
looks like a new resource. Declare identity fields on every type you check.

## Types and policies

pudl schemas are [CUE](https://cuelang.org/) definitions named
`<package>.#<Definition>`, such as `gcp.#Firewall`. Each has a `_pudl`
metadata block. There are two kinds that matter day to day.

A **type schema** (`schema_type: "base"` or `"custom"`) says what a resource
**is**. **Classification** assigns each record exactly one type: either the one
you name with `--schema` (or `populate.schema`), which the record must
satisfy, or the most specific type that inference finds. A record that matches
no type falls back to `pudl/core.#Item`. Type metadata:

| Field | Purpose |
| --- | --- |
| `identity_fields` | Fields that identify the resource |
| `sensitive_fields` | Paths replaced with `[REDACTED]` before anything is stored |
| `facts` | Projects record fields into Datalog relations |
| `owner` | Derives the owning team from labels, tags or a mapping table |
| `review` | `draft` or `validated`: whether a person or agent has confirmed the type fully represents the resource |
| `base_schema` | The parent type, for specialization |
| `secret_scan_ignore` | Paths the likely-secret scan skips |
| `@pudl(match)` | On a body field: records satisfying it claim the type, and are rejected if they fail it |

A **policy schema** (`schema_type: "policy"`) says what a **good** instance
looks like. It names its type with `base_schema` and is never assigned to a
record. On every import and observation, pudl evaluates each applicable policy
against each resource. The result is `compliant`, `noncompliant` or `unknown`
(a field the policy constrains is missing). Fields carry named rules:

```cue
sourceRanges?: _ @pudl(rule="gcp-firewall-world-ingress", severity="high")
```

`absent="violation"` on a rule treats a missing field as a violation. Results
against a draft type are marked provisional. `pudl policy list|show` reads the
stored results, and `pudl policy evaluate` re-evaluates them after a policy
changes. `pudl show ID` lists a resource's results.

See the [schema authoring guide](schema-authoring.md) to write both kinds.

## Facts and Datalog

pudl keeps a **fact store**: relations such as `gcp_firewall_network(...)`
with valid time and transaction time, so you can ask what was true and what
pudl knew at any point. Facts come from three places:

- **Projection.** A type's `_pudl.facts` turns record fields into facts.
- **Policy results.** These are stored as `policy_status` and
  `policy_violation`. The built-in rule
  `policy_noncompliant(resource, policy, severity)` serves checks.
- **The catalog.** `catalog_entry` is a built-in relation that can be joined
  in a rule body but not queried on its own.

**Rules** are Datalog written in CUE, in `.pudl/schema/pudl/rules/`. They can
be recursive, for example "everything that depends on this VPC".
`pudl query RELATION` evaluates them, and `pudl rule add|new` installs or
scaffolds them. Writing facts by hand is an advanced operation; normally you
let projection create facts and you write queries. See [facts](facts.md),
[Datalog](datalog.md) and [projection](projection.md).

## Models

A **model** is a `#SystemModel` value in CUE, under `.pudl/schema/models/`.
It is the unit `pudl check` runs:

| Field | Purpose |
| --- | --- |
| `populate` | The collector (`#CommandObserve`) |
| `observation` | The scope it covers and whether it is complete |
| `desired` | Resources you expect to exist, with expected field values |
| `checks` | Datalog queries with an expectation and a severity |
| `waivers` | Accepted exceptions to the checks' findings |
| `depends_on` | Models whose output this one reads |

A model without `populate` is **checks-only**. Its checks read what earlier
observations or imports put in the catalog. A content pack's check families
are checks-only models that read their collectors' snapshots.

`desired` produces **drift**: a desired resource that is `missing`, `changed`
(a field differs), `unidentifiable` or `ambiguous`. Drift is reported; pudl
never acts on it.

`pudl model list|show|validate|new|deps` manages models.

## Checks and findings

A **check** runs a Datalog query and expects its result to be `empty` or
`nonempty`. Its `severity` is `info`, `warn` or `fail`. Its `evidence` can
select which observations it reads (`current`, `model:NAME`, `scope:NAME` or
snapshot IDs), and `max_age` can bound their age. Each check has one
**outcome**:

| Outcome | Meaning |
| --- | --- |
| `pass` | The result matched the expectation |
| `fail` | It did not |
| `unknown` | The evidence was incomplete, missing, stale or ambiguous |
| `error` | The query could not be evaluated |
| `waived` | It failed, but waivers cover every failing row |

Each failing row is a **finding**, shown with its witness values. Findings
carry:

- **Change.** Compared with the model's earlier comparable reports, a finding
  is `new`, `persisting`, `resolved` (gone from a complete observation) or
  `unknown` (gone, but the evidence cannot show it is absent). Incomplete
  evidence never resolves a finding.
- **Owner.** Taken from the resource type's `_pudl.owner`.
  `pudl check --owner TEAM` prints one team's findings, and
  `pudl query resource_unowned` lists resources that nobody owns.

A **waiver** on a model accepts findings until a date. It targets either a
check witness (`check` and `witness`) or a policy violation (`policy`, `rule`
and `resource`), and must name an `owner`, a `reason`, and `created` and
`expires` dates. A waived finding keeps its change state; waiving never
resolves it. After `expires` the waiver stops applying and the report flags
it. A waiver that matches nothing is reported as stale.

## Reports and exit codes

`pudl check MODEL...` runs exactly the models it names, with producers first
by `depends_on`. A model whose named producer failed is recorded as `blocked`.
Each `pudl check` records one versioned **report** (`check_...`) for all of
its models, with the evidence each one used.

```bash
pudl report                      # the newest report
pudl report check_B --since check_A
pudl report check_A --json
```

The summary rates `execution`, `conformity` (drift), `checks` and
`verification` separately. Exit codes:

| Command | 0 | 1 | 2 |
| --- | --- | --- | --- |
| `pudl check` | Every model succeeded (drift alone does not fail) | A `fail` check did not pass, or a model failed or was blocked | not used |
| `pudl check --detailed-exitcode` | No drift and no failing `fail` check | A model failed or was blocked for another reason | Drift, or a `fail` check did not pass |

With `--fail-on new`, failing `fail` checks count only when they have a new
finding. `pudl status` prints each model's last recorded status.

## Doctor

`pudl doctor` checks the workspace and the catalog: schema names,
classification, identity, projected facts, policy results, file permissions,
likely secrets and sensitive values stored before a field was marked
sensitive. It names the repair for each problem. `pudl doctor --fix` applies
the repairs, `--only NAME` limits them, and `--dry-run` previews them. The
sensitive-value scrub runs only when you name it with `--only sensitive-scrub`.

## Content packs

`pudl pack install NAME` and `pudl example install NAME` copy CUE files and
fixtures into the workspace:

- `pudl pack install gcp|aws`: types, policies, collectors, rules and check
  families. See the [GCP](../packs/gcp/README.md) and
  [AWS](../packs/aws/README.md) pack READMEs.

The tutorials install with `pudl example install`:

- `gcp-network-hygiene`: a single worked firewall example. See
  [GCP network hygiene](gcp-network-hygiene.md).
- `git-inventory`: the drift tutorial in [getting started](getting-started.md).
