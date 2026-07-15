Only do exactly what is asked for when writing, refactoring or debugging code. Only make changes which directly contribute to the specific task the user has given you.

When writing or refactoring code, always keep separation of concerns, readability and maintainability in mind. Keep files under 300 lines when possible, 500 lines or so at the most. Separate code into modules with single responsibilities, and make sure to avoid circular dependencies.

When debugging code, do not delete implementations and put in placeholders. If you need to debug code by removing it from the execution path, just comment it out.

When writing code, do not add placeholder implementations unless the plan you are following or the ask from the user explicitly asks for placeholders.

When completing a task, add a file to the `implog` directory summarizing the work done, including the public API implemented. Then update the plan.md to show that youve completed the work.

## Architecture

- **Go module:** `github.com/chazu/pudl` (per go.mod; imports use the full path)
- **SQLite:** `modernc.org/sqlite` (pure Go, no CGo) via `database/sql`
- **Catalog DB:** `~/.pudl/data/sqlite/catalog.db`
- **Schema system:** CUE-based inference with heuristics + native CUE unification
- **IDs:** Content-addressed SHA256 → proquint display format
- **Schema names:** Normalized to canonical `<package>.#<Definition>` via `schemaname.Normalize()`

## Datalog & Fact Store

Pudl has a bitemporal fact store and a datalog query engine:

- **Facts table** (`facts`) — append-only, bitemporal (valid_start/valid_end + tx_start/tx_end)
- **current_facts table** — materialized view of currently-valid, non-retracted facts. Synced transactionally by AddFact/RetractFact/InvalidateFact.
- **Query orchestrator** (`internal/datalog/query.go`) — `Evaluate(db, rules, relation, constraints, scope)` is the single entry point shared by the CLI (`pudl query`) and the public API. Routes recursive-headed relations to the fixpoint evaluator, others to the SQL evaluator.
- **SQL query compiler** (`internal/datalog/compile.go`) — compiles datalog rules to parameterized SQL. Each body atom = self-join on `current_facts`/`facts` with `json_extract()` for arg access. Shared `$Variables` = equi-joins. `CompileOptions.TableOverrides` maps a relation to a backing table/view (native-column access, no `json_extract`/temporal filter).
- **Recursive evaluation** (`internal/datalog/recursive.go`) — semi-naive fixpoint via SQLite temp tables (`_rule_`, `_delta_`, `_new_`).
- **Rule partitioning** (`internal/datalog/partition.go`) — splits rules into recursive/non-recursive. SQL compiler handles non-recursive; `EvalRecursive` handles recursive.
- **Built-in EDB relation** (`internal/datalog/builtin_edb.go`) — `catalog_entry` is a built-in relation backed by the `catalog_entry_edb` view (`internal/database/catalog_entry_view.go`). Usable as a rule body atom to join facts against the catalog. The name is reserved (`database.IsReservedRelation`); `AddFact` rejects it. Querying it directly (no producing rule) errors — it is join-only.
- **Public API** (`pkg/factstore`, `pkg/eval`) — external Go consumers query stores without importing `internal/`. `factstore.Store`: `Query`, `QueryFacts`, `AddFact`, `ListCatalog`, plus `GlobalDir`/`DiscoverWorkspace`. `pkg/eval`: rule loading/parsing + `Rule/Atom/Term/Tuple`. See `docs/library-api.md`.
- **Rules:** CUE files in `~/.pudl/schema/pudl/rules/` (global) and `.pudl/schema/pudl/rules/` (repo-scoped, shadows global).

## Key Patterns

- `CatalogEntry` has nullable pointer fields for optional columns (`*string`, `*int`)
- All SQL SELECT/Scan operations must be kept in sync when adding columns
- Database migrations are idempotent (safe to run on every DB open)
- `internal/identity/` — pure functions, no DB/importer deps
- Schema metadata fields: `SchemaType`, `ResourceType`, `BaseSchema`, `IdentityFields`, `TrackedFields`, `IsListType`

## Testing

- Database tests use `os.MkdirTemp` + `NewCatalogDB(tmpDir)` pattern
- Backfill tests need DB close+reopen to trigger migration on re-open
- `CGO_ENABLED=0 go test ./...` runs all tests (no C compiler needed)
- Issue changes are exported explicitly with `br sync --flush-only`; do not rely on local database state alone.

<!-- br-agent-instructions-v1 -->

---

## Beads Workflow Integration

This project uses [beads_rust](https://github.com/Dicklesworthstone/beads_rust) (`br`/`bd`) for issue tracking. Issues are stored in `.beads/` and tracked in git.

### Essential Commands

```bash
# View ready issues (open, unblocked, not deferred)
br ready              # or: bd ready

# List and search
br list --status=open # All open issues
br show <id>          # Full issue details with dependencies
br search "keyword"   # Full-text search

# Create and update
br create --title="..." --description="..." --type=task --priority=2
br update <id> --status=in_progress
br close <id> --reason="Completed"
br close <id1> <id2>  # Close multiple issues at once

# Sync with git
br sync --flush-only  # Export DB to JSONL
br sync --status      # Check sync status
```

### Workflow Pattern

1. **Start**: Run `br ready` to find actionable work
2. **Claim**: Use `br update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: Use `br close <id>`
5. **Sync**: Always run `br sync --flush-only` at session end

### Key Concepts

- **Dependencies**: Issues can block other issues. `br ready` shows only open, unblocked work.
- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers 0-4, not words)
- **Types**: task, bug, feature, epic, chore, docs, question
- **Blocking**: `br dep add <issue> <depends-on>` to add dependencies

### Session Protocol

**Before ending any session, run this checklist:**

```bash
git status              # Check what changed
git add <files>         # Stage code changes
br sync --flush-only    # Export beads changes to JSONL
git commit -m "..."     # Commit everything
git push                # Push to remote
```

### Best Practices

- Check `br ready` at session start to find available work
- Update status as you work (in_progress → closed)
- Create new issues with `br create` when you discover tasks
- Use descriptive titles and set appropriate priority/type
- Always sync before ending session

<!-- end-br-agent-instructions -->
