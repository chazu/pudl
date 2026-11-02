# pudl documentation

## Start here

| Document | What it covers |
| --- | --- |
| [README](../README.md) | What pudl is, install, a five-minute tour, features |
| [Getting started](getting-started.md) | Tutorial: drift and evidence, then policies, checks, owners and waivers |
| [Concepts](concepts.md) | The mental model: observations, resources, types, policies, models, checks, reports |
| [Vision](VISION.md) | What pudl is for, and what is out of scope |

## Guides

| Document | What it covers |
| --- | --- |
| [Governance](governance.md) | Checks, findings, policies in checks, waivers, changes over time, owners and CI |
| [Schema authoring](schema-authoring.md) | Type and policy schemas: every `_pudl` key, and compliance policies |
| [Observation](observation-scope.md) | Collectors and fan-out, scope, completeness, failed inputs and evidence selectors |
| [Projection](projection.md) | Turning record fields into facts; sensitive fields and secret warnings |
| [Evidence](evidence.md) | Classification explanations, exact imports and exports, retention, backup and restore |
| [Dependencies and impact](cross-model-dependencies.md) | `depends_on`, check ordering and impact queries over models and resources |
| [Workspaces](workspace.md) | Workspace layout, vendored definition packages and global mode |
| [GCP network hygiene](gcp-network-hygiene.md) | A worked firewall example, from fixture to live collection |

## Reference

| Document | What it covers |
| --- | --- |
| [CLI reference](cli-reference.md) | Every command and flag, generated from the command tree |
| [Retired commands](retired-commands.md) | Removed commands, flags and model fields, with replacements |
| [Library API](library-api.md) | Go packages `pkg/factstore` and `pkg/eval` |
| [Datalog](datalog.md) | Writing rules, `pudl query`, built-in relations |
| [Facts](facts.md) | The bitemporal fact store |

## Content packs

Install with `pudl example install NAME`.

| Pack | What it covers |
| --- | --- |
| [gcp](../examples/gcp/README.md) | GCP types, policies, collectors, rules and check families |
| [aws](../examples/aws/README.md) | AWS types, policies, collectors, relationship rules and check families |
| [gcp-network-hygiene](gcp-network-hygiene.md) | One firewall type, policy and check, with a fixed fixture |
| `git-inventory` | The drift tutorial in [Getting started](getting-started.md) |

## Internals and history

These documents record how pudl is built and how it got here. Documents marked
historical describe removed features.

| Document | What it covers |
| --- | --- |
| [Architecture](architecture.md) | Storage layout, import pipeline, catalog and packages |
| [Architecture improvement report](architecture-improvement-report.md) | Historical: the 2026-07 improvement recommendations, all delivered |
| [Plan](plan.md) | What has been built and what comes next |
| [Testing](TESTING.md) | Test architecture, smoke tests and benchmarks |
| [Inference algorithm](inference-algorithm.md) | How classification scores and unifies candidate types |
| [Vestige sweep](vestige-sweep.md) | Audit of leftovers from earlier designs |
| [System models build status](system-models-build-status.md) | Historical: the convergence-era model runner |
| [mu integration](mu-integration.md) | Historical: the pudl and mu collaboration, removed 2026-11-01 |
| [mu, pudl and Batcave user experience](mu-pudl-batcave-user-experience.md) | Historical: operator feedback from the mu era |
| [GCP cataloging findings](gcp-cataloging-workflow-findings.md) | Findings that shaped the cataloging workflow (addressed) |
| [design/](design/) | Design reports and decisions, each with its status |
| [research/](research/), [issues/](issues/) | Research notes and known gaps |
| [implog/](../implog/) | Implementation logs, one per delivery |
