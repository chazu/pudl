# Datalog Evaluator

PUDL includes a Datalog evaluator that derives new facts from existing ones using inference rules. It reads base facts (EDB) from both the [bitemporal fact store](facts.md) and the catalog, evaluates rules to a fixed point, and returns derived facts (IDB).

## How It Works

Rules are compiled to **parameterized SQL** and executed directly inside SQLite. Each body atom becomes a self-join on the `current_facts` table (or `facts` for temporal queries). Argument access uses JSON text extraction and a checked numeric conversion before SQLite evaluates joins or filters. Shared variables across body atoms become equi-join conditions. Ground terms become WHERE predicates.

A query is **goal-directed**: only the queried relation's dependency closure
(the derived relations it transitively reads) is evaluated. Rules the query
never reads are not run, however expensive or deep their recursion.

The closure is split into strongly connected components — groups of relations
that read each other — in dependency order. A component is **cyclic** when it
has more than one relation or a relation that reads itself.

- **No cycles** (the common case): the whole query compiles to one SQL
  statement. Each derived relation becomes a common table expression in
  dependency order, and later relations join earlier ones. A relation derived
  by several rules is their `UNION`, so a tuple derived twice is returned once.
- **Cycles** (e.g., transitive closure): components are materialized into
  SQLite temp tables in dependency order. An acyclic component is one
  `INSERT`; a cyclic component uses semi-naive fixpoint evaluation:

  1. Create temp tables `_rule_<relation>`, `_delta_<relation>`, `_new_<relation>`
  2. Seed with the component's base rules (bodies that read no member)
  3. Loop: join delta against data, insert new rows, rebuild delta
  4. Stop when no new rows are produced (fixed point)
  5. Extract results; the transaction rolls back, dropping the temp tables

All iteration happens inside SQLite. Only final results cross the SQL/Go boundary.

## Writing Rules

Rules are CUE files containing fields with `head` and `body` structure. The head is the derived fact pattern; the body is a list of conditions. Variables use `$`-prefix convention.

### Example: Transitive Dependencies

```cue
baseDep: {
    name: "base_dep"
    head: { rel: "depends_transitive", args: { from: "$X", to: "$Z" } }
    body: [{ rel: "depends", args: { from: "$X", to: "$Z" } }]
}

recursiveDep: {
    name: "recursive_dep"
    head: { rel: "depends_transitive", args: { from: "$X", to: "$Z" } }
    body: [
        { rel: "depends",            args: { from: "$X", to: "$Y" } },
        { rel: "depends_transitive", args: { from: "$Y", to: "$Z" } },
    ]
}
```

Given facts `depends(api, db)` and `depends(db, cache)`, this derives `depends_transitive(api, db)`, `depends_transitive(db, cache)`, and `depends_transitive(api, cache)`.

### Example: Flagging Obstacles

```cue
obstacleAlert: {
    name: "obstacle_alert"
    head: { rel: "at_risk", args: { scope: "$S" } }
    body: [{ rel: "observation", args: { kind: "obstacle", scope: "$S" } }]
}
```

Any observation with `kind=obstacle` produces a derived `at_risk` fact for that scope.

### Example: Cross-Relation Join

```cue
flaggedOrigin: {
    name: "flagged_origin"
    head: { rel: "flagged", args: { origin: "$O" } }
    body: [
        { rel: "observation",    args: { kind: "obstacle", scope: "$S" } },
        { rel: "catalog_entry",  args: { origin: "$O", schema: "$S" } },
    ]
}
```

This joins observations against catalog entries, finding origins that have obstacles flagged for their schema.

### Rule Structure

```cue
// #Rule schema (pudl/rules package)
#Rule: {
    name?: string           // optional, used for shadowing and display
    head:  #Atom            // the derived fact pattern
    body:  [...#Atom]       // conditions (at least one required)
}

#Atom: {
    rel:  string            // relation name
    args: {[string]: #Term} // named arguments
}

#Term: string | number | bool  // $-prefixed strings are variables
```

**Variables** (`$X`, `$Y`, `$Z`) are unified across body atoms. If `$X` appears in two body atoms, they must bind to the same value.

**Ground terms** (`"obstacle"`, `42`, `true`) match only the exact value.

**Constant head arguments** are part of every derived tuple and can be
filtered on like any other key. `head: {rel: "flagged", args: {id: "$X",
severity: "high"}}` derives `flagged(id=..., severity=high)`, and
`pudl query flagged severity=high` matches it.

**Head keys** are a relation's columns. Every rule deriving the same relation
must use the same set of head keys; a mismatch is reported by `pudl doctor`
and fails any query that reads the relation.

**Argument keys** may be quoted CUE labels (`"app.kubernetes.io/name"`). The
key is the unquoted label, and a key containing `.` or `/` names one top-level
argument, not a nested path. Keys may not contain a double quote.

**Aggregates** (`count($X)`, `sum`, `min`, `max`) are head-only. They may read
any derived relation, including the result of a recursive one, as long as the
aggregating rule is not itself part of a cycle.

### Rule validity

A top-level struct field with a `head` or a `body` is a rule; other fields are
ignored. A rule that cannot be read as written — a missing `rel`, an empty body,
a head variable not bound by the body (range restriction), an aggregate in the
body — is **not skipped**. It is loaded as invalid, with its source position,
and:

- `pudl rule add` refuses the file and lists each problem;
- `pudl rule new` warns about invalid rules already in the rules directory;
- `pudl doctor` reports every invalid rule under **Datalog Rules**;
- `pudl query` and model checks fail when the queried relation depends on an
  invalid rule. An invalid rule whose head relation cannot be read blocks every
  query, since it might derive anything.

Silently dropping a broken rule would make queries that depend on it return
nothing, which a model check would read as a pass.

### Numeric values

Queries preserve signed 64-bit integers, including values above `2^53`, across
base, derived, recursive, and historical results. Integer spellings with a
decimal point or exponent normalize before comparison. Non-integral decimals
must round-trip through `float64` and Go JSON serialization without changing
their decimal value. Unsupported numeric operands and accessed values fail
with `unsupported query number`; raw evidence remains available through
`pudl facts list` or `Store.QueryFacts`.

For example, `9007199254740992` and `9007199254740993` stay distinct, while
`9007199254740993` and `9.007199254740993e15` compare equally. See the
[library numeric contract](library-api.md#numeric-query-contract) for the range,
Go result types, and arithmetic limits.

The compiler uses SQLite's [`->` operator](https://www.sqlite.org/json1.html#the_and_operators)
to obtain the JSON token before `pudl_query_value` checks it. Using
`json_extract` directly would convert the token to INTEGER/REAL before PUDL
could detect lost precision. PUDL registers this function on its catalog
connections. Direct SQLite clients reading PUDL query projections need that
function.

## Where Rules Live

Rules follow PUDL's workspace scoping pattern:

```
~/.pudl/schema/pudl/rules/    Global rules (apply everywhere)
.pudl/schema/pudl/rules/      Repo-scoped rules (apply to this repo only)
```

Repo-scoped rules shadow global rules with the same `name` field.

## CLI Commands

### `pudl query`

Evaluate rules and query results:

```bash
# Query a derived relation
pudl query depends_transitive

# With constraints (key=value pairs)
pudl query depends_transitive from=api

# Query base facts directly (works without rules)
pudl query observation kind=obstacle

# Load ad-hoc rules from a file (in addition to stored rules)
pudl query at_risk -f my-analysis.cue

# Machine-readable output
pudl query depends_transitive --json
```

Numeric constraints are parsed without rounding. Preserve JSON quotes to match
a numeric-looking string instead:

```bash
pudl query numbers n=9007199254740993 --json
pudl query numbers n=9.007199254740993e15 --json
pudl query numbers 'n="9007199254740993"' --json
```

Rules are compiled to SQL, executed, and results filtered by the requested relation and constraints. Temporal flags switch from `current_facts` to the full `facts` table with time-scoped filters.

| Flag | Description |
|------|-------------|
| `-f, --rule-file` | Load additional rules from a CUE file |
| `--as-of-valid` | Evaluate over facts true at this time (RFC3339 or Unix) |
| `--as-of-tx` | Evaluate over facts known at this time (RFC3339 or Unix) |
| `--all-workspaces` | Include global rules and all workspace data |
| `--max-iterations` | Cap on fixpoint rounds per recursive cycle (default 100) |
| `--json` | Output as JSON |

### `pudl rule add`

Validate and install a rule file:

```bash
# Install to repo-scoped rules
pudl rule add transitive-deps.cue

# Install to global rules
pudl rule add company-standards.cue --global
```

The file is validated before installation -- it must parse as valid CUE, contain at least one field with `head` and `body`, and every rule in it must be valid (see [Rule validity](#rule-validity)). On success, the command reports what rules were installed and where:

```
Installed 2 rule(s) from transitive-deps.cue (repo-scoped)
  base_dep: depends_transitive :- depends
  recursive_dep: depends_transitive :- depends, depends_transitive
Location: .pudl/schema/pudl/rules/transitive-deps.cue
```

| Flag | Description |
|------|-------------|
| `--global` | Install as a global rule |

## EDB Sources

The evaluator reads base facts from two sources.

### Fact Store

For present-time queries, the SQL compiler reads from the `current_facts` table -- a materialized view of only currently-valid, non-retracted facts. For temporal queries (`--as-of-valid`, `--as-of-tx`), it reads from the full `facts` table with appropriate temporal filters. Any relation name not reserved as a built-in (below) is read from the fact store.

### Catalog (`catalog_entry`)

The catalog is exposed to Datalog as a built-in `catalog_entry` relation, backed by the `catalog_entry_edb` SQL view over `catalog_entries`. Because the view has native columns (not a JSON `args` blob), the compiler reads its columns directly via `CompileOptions.TableOverrides` -- no `json_extract`, and no temporal filtering (the catalog is atemporal).

Available fields (view columns):

| Field | Source |
|-------|--------|
| `id` | Entry ID |
| `schema` | CUE schema name |
| `origin` | Data origin / workspace |
| `format` | File format |
| `status` | Convergence status |
| `entry_type` | import, observe, manifest, manifest-action |
| `target` | mu target / run target name (e.g. `//models/<name>`, `home/odroid`) |
| `run_id` | Run identifier |
| `resource_id` | Stable resource identity |
| `content_hash` | SHA256 of stored data |
| `version` | Monotonic version per `resource_id` |
| `collection_id` / `collection_type` / `item_id` | Collection membership |

Rules can join facts against the catalog -- e.g., matching an observation against the catalog entry it refers to:

```cue
owned: {
    head: { rel: "owned", args: { id: "$I", team: "$T" } }
    body: [
        { rel: "catalog_entry", args: { id: "$I", origin: "$O" } },
        { rel: "team_owns",     args: { origin: "$O", team: "$T" } },
    ]
}
```

**`catalog_entry` is join-only and reserved:**

- It works as a rule **body atom**, not as a direct query target. `pudl query catalog_entry` (no rule producing it) returns a clear error, not a silent empty result. To list catalog entries, use `pudl list` or the library `Store.ListCatalog` (see [library-api.md](library-api.md)).
- The name is reserved: `AddFact` rejects facts asserted under the `catalog_entry` relation, so user facts can never silently shadow the built-in.

## Temporal Queries

All Datalog evaluation respects bitemporal semantics. By default, rules evaluate over **current facts** (valid now, not retracted). Temporal flags shift the evaluation window.

### What Was True At a Point In Time

```bash
# What observations were valid at deploy time?
pudl query observation --as-of-valid 2026-04-01T14:30:00Z

# What dependencies existed last month?
pudl query depends_transitive --as-of-valid 2026-04-15T00:00:00Z
```

When `--as-of-valid` is set, the compiler switches from `current_facts` to the full `facts` table with:
```sql
WHERE valid_start <= ? AND (valid_end IS NULL OR valid_end > ?)
  AND tx_end IS NULL
```

This answers "what was true at time T, according to our **current** knowledge" -- if a fact was later retracted (we learned it was wrong), it won't appear.

### What We Believed At a Point In Time

```bash
# What did we know last Tuesday?
pudl query observation --as-of-tx 1743379200
```

When `--as-of-tx` is set:
```sql
WHERE tx_start <= ? AND (tx_end IS NULL OR tx_end > ?)
```

This answers "what did we believe at time T" -- includes facts that were later retracted (because we hadn't retracted them yet at that point).

### Combined: What We Believed About What Was True

```bash
# What did we believe on May 1st about what was true on April 15th?
pudl query observation --as-of-valid 2026-04-15T00:00:00Z --as-of-tx 2026-05-01T00:00:00Z
```

Both filters apply simultaneously. Useful for reconstructing past decision states.

### How Temporal Scope Propagates

Temporal flags apply **globally** to the entire rule evaluation. Every body atom in every rule sees the same temporal window. This means:

- Recursive rules (transitive closure) compute the closure as it existed at the specified time
- Cross-relation joins work correctly -- both sides see the same temporal snapshot
- Derived facts inherit the temporal semantics of their input facts

There is no per-atom temporal override -- all atoms in a rule evaluation share one temporal scope. This keeps semantics simple and results consistent.

### Relationship to `pudl facts list`

`pudl facts list` queries raw facts without rule evaluation. `pudl query` evaluates rules. Both support the same temporal flags:

| Command | Evaluates Rules | Temporal Flags |
|---------|-----------------|----------------|
| `pudl facts list --relation X` | No | `--as-of-valid`, `--as-of-tx` |
| `pudl query X` | Yes | `--as-of-valid`, `--as-of-tx` |

For details on fact lifecycle (retraction vs invalidation) and the bitemporal model, see [facts.md](facts.md).

## Performance

Recursive rules may contain multiple derived body atoms, including nonlinear
self-joins such as `reach(X,Z) :- reach(X,Y), reach(Y,Z)`. Each round evaluates
one variant per derived occurrence: that occurrence reads newly derived tuples,
and the others read accumulated tuples. The next delta excludes tuples already
known. This also handles derived inputs that become available in different
rounds, and applies identically to current and historical queries. Aggregation
inside a cycle remains unsupported.

Rules compile to SQL, so SQLite's query planner handles join ordering and index selection. The `current_facts` table is indexed on `relation` for fast base-case lookups. Recursive evaluation uses temp tables with primary key dedup, avoiding redundant re-derivation.

Because evaluation is goal-directed, a query that matches nothing — the common
case for a passing model check — costs one SQL statement, even in a workspace
with recursive rules. Joins on fact arguments are not indexed (each argument is
read from JSON per row), so large joins over base facts dominate cost.

Each cyclic component iterates at most 100 rounds by default; each round
extends a recursive derivation by one step, so the limit bounds the longest
chain a recursive rule can follow. Raise it with `pudl query --max-iterations`.
Model checks and `factstore.Store.Query` use the default.
