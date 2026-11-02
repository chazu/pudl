# pudl

pudl helps SREs and coding agents discover, catalog and govern cloud
infrastructure. It runs the read-only commands you already use (`gcloud`,
`aws`, `kubectl` with JSON output), or imports files you have. It stores every
record with its evidence, tracks each resource across observations, and
reports what violates your policies and checks.

pudl observes and reports. It never changes infrastructure: Terraform,
`kubectl` or whatever tool owns a system makes changes, and the next pudl check
verifies them.

pudl is one binary. Each repository gets its own `.pudl/` workspace: CUE
schemas and models you commit, and a local SQLite catalog that stays out of
Git.

## Install

You need Go 1.26.10 or newer. Install the current development version and put
Go's binary directory (normally `~/go/bin`) on your `PATH`:

```bash
go install github.com/chazu/pudl@main
pudl version
```

## A five-minute tour

The GCP content pack ships offline fixtures, so this tour needs no cloud
account. Run it in a new directory:

```bash
mkdir pudl-tour && cd pudl-tour
pudl init
pudl example install gcp
```

`pudl init` creates `.pudl/`. The pack installs draft GCP schemas,
best-practice policies, collectors and check families.

Check the network hygiene family together with the fixture collectors it reads:

```bash
pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers \
  gcp-fixture-network-hygiene-checks
```

The three collectors observe first. Each one records a snapshot (`snap_...`)
marked `complete: true`. Then the checks-only family evaluates its checks over
those snapshots. The fixtures include bad resources on purpose, so the report
lists findings and the command exits 1:

- `policy-high` fails on a firewall that allows ingress from `0.0.0.0/0`.
- `policy-any` (severity `warn`) also lists two unused external addresses.
- `nat-ip-not-in-use` finds a Cloud NAT that lists an address GCP reports as
  `RESERVED`.

Each finding is `new` because no earlier report exists. Run the same command
again and they become `persisting`. `pudl report` prints the newest report
again, and `--json` gives the same report for scripts.

Next, see which policies were evaluated and which resources fail them:

```bash
pudl policy list
pudl policy show gcp.#FirewallBestPractice
```

Then inspect the failing firewall. Its entry ID comes from its content, so it
is the same in every workspace:

```bash
pudl show vomap-foqab
pudl show vomap-foqab --history
```

`pudl show` prints the entry's type (`gcp.#Firewall`) and its compliance
section; add `--raw` to see the record itself. `--history` lists every
observation of that resource.

Finally, gate on the result as CI would:

```bash
pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers \
  gcp-fixture-network-hygiene-checks --fail-on new --detailed-exitcode
echo $?
```

The exit status is 0: the findings still fail, but none is new.

Next steps: the [tutorial](docs/getting-started.md) walks through drift,
evidence, waivers and ownership. To collect from a real project, follow the
[GCP pack README](examples/gcp/README.md).

## How it fits together

```
 gcloud / aws / kubectl ... --format=json      pudl import --path FILE
                  |                                       |
                  +------------------+--------------------+
                                     v
   observation -> snapshot (scope, complete)  -> catalog entries
                                                 typed by CUE schemas
                                                 tracked as resources
                                     |
             +-----------------------+------------------------+
             v                       v                        v
     policies per resource     projected facts          desired state
     (compliant/noncompliant)  + Datalog rules          (drift)
             |                       |                        |
             +-----------> checks on a model <----------------+
                                     |
                         findings: new / persisting / resolved,
                         owner, waivers  ->  report + exit code
```

- **Observe.** A model's `populate` runs commands that print JSON, or you run
  `pudl import`. Each observation is a snapshot with a scope and a completeness
  flag.
- **Catalog.** Each record is a catalog entry. A type schema classifies it,
  identity fields make it a resource, and pudl keeps its history.
- **Govern.** Policies describe what a good resource looks like. Checks are
  Datalog queries over facts. A `pudl check` run produces one report with
  findings, owners and waivers.

The [concepts guide](docs/concepts.md) explains each term.

## Features

**Discover**
- Collectors (`#CommandObserve`) run plain commands with no shell, fan out per
  project, account or region, and can enumerate inputs at run time (`each`).
  A failed input makes the observation incomplete; it never makes resources
  look deleted.
- `pudl import` reads JSON, YAML, CSV and NDJSON from files, directories or
  stdin, including `.gz` and `.zst` files.
- Content packs for GCP and AWS, a GCP network hygiene example and a Git
  inventory tutorial: `pudl example install gcp|aws|gcp-network-hygiene|git-inventory`.

**Catalog**
- Content-addressed entries with short IDs (`vomap-foqab`), stable resource
  IDs from schema identity fields, and per-resource history.
- CUE type schemas with inference or an explicit `--schema`. Sensitive fields
  are redacted before storage.
- Snapshots you can list, pin and prune (`pudl snapshot`), filters
  (`pudl list --where`, `--select`), exports and portable workspace bundles
  (`pudl export`).

**Explore**
- `pudl ui` opens a graph explorer, embedded in the binary, on the live
  catalog: network, application, inventory and security views of the same
  resources, with evidence for every relationship. `pudl graph export` writes
  the same graph as JSON. See [frontend](docs/frontend.md).

**Govern**
- Policy schemas evaluated per resource on every observation, with named rules
  and severities (`pudl policy list|show|evaluate`).
- Datalog rules written in CUE over projected facts, the catalog and policy
  results (`pudl query`, `pudl rule`).
- Models with desired state (drift) and checks. Findings are tracked as new,
  persisting, resolved or unknown, carry owners, and can be waived until a
  date.
- One versioned report per check (`pudl report`), comparison between reports
  (`--since`) and CI exit codes (`--detailed-exitcode`, `--fail-on new`).
- `pudl doctor` diagnoses the catalog, and `pudl doctor --fix` repairs it.

**Agents**
- `pudl prime` prints an agent prompt, and `pudl guide` prints topic guides.
  `pudl init` installs a Claude skill in `.claude/skills/`.
- Every command takes `--json`, and `pudl help --json` prints the command tree.
- Go programs can query a workspace through `pkg/factstore` and `pkg/eval`.

## Documentation

- [Getting started](docs/getting-started.md): the hands-on tutorial.
- [Concepts](docs/concepts.md): the mental model.
- [CLI reference](docs/cli-reference.md): every command and flag.
- [Documentation index](docs/README.md): guides, reference, content packs and
  project history.

Commands that were removed, such as `pudl run` and convergence through mu, are
listed with their replacements in [retired commands](docs/retired-commands.md).

## Requirements

- Go 1.26.10 or newer to build.
- Git, to version your schemas and models.
- The CLIs your collectors call (`gcloud`, `aws`, `kubectl`), for live
  collection only.
