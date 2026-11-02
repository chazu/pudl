# In-CLI guidance rewritten: `pudl guide`, `pudl prime`, pudl-core skill

`pudl guide`, `pudl prime` and the embedded `pudl-core` skill now describe
pudl as a tool that discovers, catalogs and governs infrastructure, in the
terms of the docs rewrite brief (observation, snapshot, resource, type
schema, policy, model, check, finding, waiver, report).

## What changed

- **`pudl guide`.** Topics are now overview, observe, schemas, policies,
  checks, query, doctor, packs and agents. `import` and `models` merged into
  `observe` and `checks`; `facts` and `datalog` into `query`;
  `troubleshooting` into `doctor`. Topic text moved out of Go into one embedded
  file per topic (`cmd/guide/<topic>.txt`, `//go:embed`). `guideTopics` is an
  ordered list of name and summary that drives the index and completion.
  Unknown topics still exit 2.
- **`pudl prime`.** A short agent briefing: what pudl is, the loop (init →
  pack or collector → check → report, policy list/show, show → fix or waive →
  re-check with `--fail-on new`) matching the README tour, the rules that
  matter (named models only, unknown is not pass, change states), JSON
  contracts and exit codes, everyday commands, retired commands, and pointers.
- **Skill.** `skills/pudl-core/SKILL.md` rewritten to match: agent workflow,
  JSON contracts, collector, check and waiver CUE, the `_pudl` block, the
  policy-authoring gotchas, query and rules, doctor, and a "do not use" list.
- **Help text.** `pudl facts` help points at `pudl guide query` and no longer
  lists the retired `reproject` subcommand; `pudl prime` help drops "data
  lake". Golden help and `docs/cli-reference.md` regenerated. The smoke test
  reads `pudl guide query` instead of the removed `facts` topic.

## Line counts

| Surface | Before | After |
| --- | --- | --- |
| `pudl guide` (`cmd/guide.go` + topic text) | 643 | 622 (90 Go + 532 text) |
| `pudl prime` (`cmd/prime.go`) | 123 | 108 |
| skill (`skills/pudl-core/SKILL.md`) | 249 | 189 |

`cmd/guide_test.go` (39 lines) is new: every listed topic has a text file
with the right heading, every file is listed, and completion offers the
listed topics.

## Accuracy gate

A throwaway script extracted every `pudl <subcommand>` and `--flag` from the
guide, prime and the skill and checked each against `pudl help --json`; it
ends with 0 mismatches across 37 commands. Every offline-runnable example was
then run in a temporary workspace with all four packs installed (98 steps,
including the collector, schema, check, waiver and rule snippets written as
files). Fixed along the way:

- `pudl schema new --path <pkg>/#<Def>` fails; the format is `<pkg>:#<Def>`
  (also wrong in the previous guide, prime and skill).
- A saved check (`model new --check`) whose evidence does not cover every
  relation its query reads is unknown; the example now passes both
  collectors' evidence and says so.
- A witness waiver must name a check of its own model; the example says so.
- `pudl facts search "prod-nat"` fails (see below); the example searches a
  plain word.

Product issues found, not fixed here:

- Importing a JSON array (`pudl import fw.json`, with or without `--schema`)
  stores a `pudl/core.#Collection` entry that `pudl doctor` reports invalid,
  so doctor fails and every `doctor --fix` (including `--dry-run` and
  `--only`) exits 1 and prints usage.
- Two collectors whose records are byte-identical: the second fails with
  `UNIQUE constraint failed: catalog_entries.id`.
- `pudl facts search` passes text to FTS unescaped: a hyphen gives
  `SQL logic error: no such column`.
- Without `--schema`, the GCP pack's firewall and route fixtures import as
  `pudl/infra.#Service`.

## Public API

No Go API change. `cmd`: `guideTopics []guideTopic` (ordered name and
summary), `guideTopicText(name string) (string, error)`.
