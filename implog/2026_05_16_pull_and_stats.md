# Pull Command and Facts Stats

**Date:** 2026-05-16

## What Was Done

Added two new CLI commands and expanded temporal documentation.

### `pudl pull` — Entity/Scope Retrieval

New command for retrieving all facts related to a scope or entity with prefix matching.

```bash
pudl pull procyon-park:src/cli      # scope prefix match
pudl pull --kind bug                # filter by kind
pudl pull --source claude-code      # filter by source
pudl pull procyon-park --kind bug   # combined filters
pudl pull maggie:vm --json          # JSON output
```

Output grouped by scope with description, kind, source, and date.

**File:** `cmd/pull.go`

### `pudl facts stats` — Aggregate Statistics

New subcommand for grouped counts over the fact store.

```bash
pudl facts stats                                # count by relation
pudl facts stats --relation observation         # count by kind
pudl facts stats --group-by scope              # count per scope
pudl facts stats --group-by kind,source        # cross-tabulation
```

Supports any JSON arg field as a group-by dimension. Outputs aligned table or JSON.

**File:** `cmd/facts.go` (extended)

### Temporal Query Documentation

Added comprehensive "Temporal Queries" section to `docs/datalog.md` explaining:
- `--as-of-valid` semantics (what was true at time T)
- `--as-of-tx` semantics (what we believed at time T)
- Combined mode
- How temporal scope propagates through rule evaluation
- Relationship between `pudl facts list` and `pudl query`

## Public API

```
pudl pull [scope] [--kind X] [--source X] [--relation X] [--json]
pudl facts stats [--relation X] [--group-by field1,field2] [--json]
```

## Files Changed

- Created: `cmd/pull.go`
- Modified: `cmd/facts.go` (added stats subcommand)
- Modified: `docs/datalog.md` (temporal queries section)
- Modified: `docs/cli-reference.md` (pull + stats docs)
