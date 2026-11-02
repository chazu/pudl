# Fact store

Facts are what Datalog rules, `pudl query` and model checks read. Each fact is
a **relation** name with named **args** (a JSON object), a **source**, and two
time ranges. Facts live in the same SQLite database as the catalog.

## Where facts come from

You rarely write facts by hand. pudl writes them as it works:

| Facts | Written by |
|-------|------------|
| Relations a schema declares in `_pudl.facts` | Every import and observation ([projection](projection.md)) |
| `resource_owner`, `resource_unowned` | Projection, from `_pudl.owner` |
| `policy_status`, `policy_violation` | Policy evaluation, after every import and observation, and `pudl policy evaluate` |
| `model_depends_on` | `pudl check` and `pudl model deps` ([dependencies](cross-model-dependencies.md)) |

Projected facts always describe a resource's most recent observation. When a
later observation changes a resource, its old facts are invalidated, so
history shows the change.

## Read facts

```bash
pudl query gcp_address status=RESERVED          # evaluate rules and filter (see datalog.md)
pudl facts list --relation gcp_address          # stored facts, no rules
pudl facts list --relation gcp_address --verbose
pudl facts show c0b4392d347a                    # one fact, by ID or unique prefix
pudl facts search RESERVED                      # full-text search over current facts
pudl facts stats                                # counts per relation
```

`facts search` uses SQLite FTS5 syntax over the values of current facts:
bare terms are ANDed, `"quoted phrases"` match in order, and a trailing `*`
matches a prefix. A term with punctuation, such as `prod-nat` or `10.0.0.1`,
is matched as a phrase. `facts stats` counts by relation unless `--group-by` names
`source` or arg fields; several comma-separated fields cross-tabulate.

## Time travel

Every fact has two independent time axes:

- **Valid time** (`valid_start`, `valid_end`): when the fact was true in the
  world. A firewall rule was open from Monday to Wednesday, whenever pudl
  learned about it.
- **Transaction time** (`tx_start`, `tx_end`): when the store believed it. A
  fact recorded Monday and retracted as a mistake on Wednesday was believed
  from Monday to Wednesday.

| Question | Flags |
|----------|-------|
| What is true now? | none |
| What was true at T, by what we know now? | `--as-of-valid T` |
| What did the store believe at T? | `--as-of-tx T` |
| What did we believe at T2 about T1? | `--as-of-valid T1 --as-of-tx T2` |
| What did the store hold right after write N? | `--as-of-tx-seq N` (`facts list` only) |

`pudl facts list` and `pudl query` both take the time flags. Times are RFC3339
or Unix seconds.

Times are whole seconds. `--as-of-tx T` means the state after every write
committed during or before second T, so a fact recorded and retracted within
one second never appears. Every write also gets a store-wide sequence number;
`--as-of-tx-seq N` selects the state right after write N exactly. Sequence
numbers are ordered but can skip.

## Retract or invalidate

Two operations end a fact, with different meanings:

- **Retract**: "this record was wrong." It ends the store's belief
  (`tx_end`). The fact leaves current queries and `--as-of-valid` queries, but
  `--as-of-tx` queries for earlier moments still show it.
- **Invalidate**: "this was true, and is not any more." It records a new
  version of the fact whose `valid_end` is now. The fact leaves current
  queries but stays in `--as-of-valid` queries for the time it was true.

Projection and dependency recording invalidate facts when the world changes.
Projection retracts facts that were corrections: a changed facts block, a
deleted or re-identified entry, or a record reassigned to another schema.

History is append-only. Invalidation never edits the old row: it ends that
version's belief and inserts a successor version with `valid_end` set and
`supersedes` naming the old ID. An old ID still resolves: `pudl facts show ID`
shows the newest version, and `--exact` shows the version the ID names.
Retracting or invalidating an old ID acts on its newest version. A retracted
fact cannot be invalidated.

## Fact IDs

A fact's ID is a SHA256 of its relation, canonical args, `valid_start` and
source. Canonical args sort object keys and normalize number spellings
without rounding, so `1`, `1.0` and `1e0` are the same, while
`9007199254740992` and `9007199254740993` stay distinct. Consequently:

- the same source recording the same fact at the same time deduplicates;
- different sources recording the same fact keep separate IDs, so
  corroboration is visible;
- writing an existing ID again returns the stored fact unchanged and does not
  revive a retracted or invalidated one;
- reusing an ID for different content is an error.

The successor version an invalidation creates has the ID
`SHA256("supersedes" \x00 old_id \x00 valid_end)`.

## Write facts (advanced)

Writing facts by hand is for integrations and agents, not the everyday path.
The commands are hidden from `pudl --help` but documented under
[advanced commands](cli-reference.md#advanced-commands):

```bash
pudl facts add --relation service_tier --args '{"service":"api","tier":"1"}' --source sre
pudl facts add --relation config --args '{"key":"timeout","value":30}' --schema user/config.#Setting
pudl facts retract c0b4392d347a
pudl facts invalidate c0b4392d347a
```

`--args` must be a JSON object. `--source` defaults to the OS user. `--schema`
validates the args against a CUE definition first. A likely secret in the
args is reported as a warning; facts are never redacted. The reserved
relations `catalog_entry`, `policy_status` and `policy_violation` cannot be
written.

Go programs use `pkg/factstore`, which also offers atomic check-and-write
transactions; see the [library API](library-api.md).

## Storage

| Column | Meaning |
|--------|---------|
| `id` | Content-addressed ID |
| `relation`, `args` | The fact; `args` is a JSON object |
| `valid_start`, `valid_end` | Valid time, Unix seconds; `valid_end` is NULL while true |
| `tx_start`, `tx_end` | Transaction time; `tx_end` is NULL while believed |
| `tx_seq`, `tx_end_seq` | Write sequence numbers that created and ended this version |
| `supersedes` | The version an invalidation replaced |
| `source`, `provenance` | Who asserted it, and optional JSON context |

The `facts` table holds every version. `current_facts` holds the facts valid
and believed now, kept in sync by every write, and current queries read it.
