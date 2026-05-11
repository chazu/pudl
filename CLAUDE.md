Only do exactly what is asked for when writing, refactoring or debugging code. Only make changes which directly contribute to the specific task the user has given you.

When writing or refactoring code, always keep separation of concerns, readability and maintainability in mind. Keep files under 300 lines when possible, 500 lines or so at the most. Separate code into modules with single responsibilities, and make sure to avoid circular dependencies.

When debugging code, do not delete implementations and put in placeholders. If you need to debug code by removing it from the execution path, just comment it out.

When writing code, do not add placeholder implementations unless the plan you are following or the ask from the user explicitly asks for placeholders.

When completing a task, add a file to the `implog` directory summarizing the work done, including the public API implemented. Then update the plan.md to show that youve completed the work.

## Architecture

- **Go module:** `pudl` (not `github.com/...`)
- **SQLite:** `modernc.org/sqlite` (pure Go, no CGo) via `database/sql`
- **Catalog DB:** `~/.pudl/data/sqlite/catalog.db`
- **Schema system:** CUE-based inference with heuristics + native CUE unification
- **IDs:** Content-addressed SHA256 → proquint display format
- **Schema names:** Normalized to canonical `<package>.#<Definition>` via `schemaname.Normalize()`

## Datalog & Fact Store

Pudl has a bitemporal fact store and a datalog query engine:

- **Facts table** (`facts`) — append-only, bitemporal (valid_start/valid_end + tx_start/tx_end)
- **current_facts table** — materialized view of currently-valid, non-retracted facts. Synced transactionally by AddFact/RetractFact/InvalidateFact.
- **SQL query compiler** (`internal/datalog/compile.go`) — compiles datalog rules to parameterized SQL. Each body atom = self-join on `current_facts`/`facts` with `json_extract()` for arg access. Shared `$Variables` = equi-joins.
- **Recursive evaluation** (`internal/datalog/recursive.go`) — semi-naive fixpoint via SQLite temp tables (`_rule_`, `_delta_`, `_new_`).
- **Rule partitioning** (`internal/datalog/partition.go`) — splits rules into recursive/non-recursive. SQL compiler handles non-recursive; `EvalRecursive` handles recursive.
- **In-memory evaluator** (`internal/datalog/eval.go`) — legacy, kept for reference but no longer used in the query path.
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
- Pre-commit hook (`bd hook`) may be broken — use `--no-verify` if needed
