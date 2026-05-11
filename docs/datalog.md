# Datalog Evaluator

PUDL includes a Datalog evaluator that derives new facts from existing ones using inference rules. It reads base facts (EDB) from both the [bitemporal fact store](facts.md) and the catalog, evaluates rules to a fixed point, and returns derived facts (IDB).

## How It Works

Rules are compiled to **parameterized SQL** and executed directly inside SQLite. Each body atom becomes a self-join on the `current_facts` table (or `facts` for temporal queries), with `json_extract()` for arg access. Shared variables across body atoms become equi-join conditions. Ground terms become WHERE predicates.

**Non-recursive rules** are compiled to a single SQL query per rule. Multiple rules deriving the same relation are combined with UNION ALL.

**Recursive rules** (e.g., transitive closure) use semi-naive fixpoint evaluation via SQLite temp tables:

1. Create temp tables `_rule_<relation>` and `_delta_<relation>`
2. Seed with base case results
3. Loop: join delta against data, insert new rows, rebuild delta
4. Stop when no new rows are produced (fixed point)
5. Extract results and drop temp tables

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

Rules are compiled to SQL, executed, and results filtered by the requested relation and constraints. Temporal flags switch from `current_facts` to the full `facts` table with time-scoped filters.

| Flag | Description |
|------|-------------|
| `-f, --rule-file` | Load additional rules from a CUE file |
| `--as-of-valid` | Evaluate over facts true at this time (RFC3339 or Unix) |
| `--as-of-tx` | Evaluate over facts known at this time (RFC3339 or Unix) |
| `--all-workspaces` | Include global rules and all workspace data |
| `--json` | Output as JSON |

### `pudl rule add`

Validate and install a rule file:

```bash
# Install to repo-scoped rules
pudl rule add transitive-deps.cue

# Install to global rules
pudl rule add company-standards.cue --global
```

The file is validated before installation -- it must parse as valid CUE and contain at least one field with `head` and `body`. On success, the command reports what rules were installed and where:

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

The evaluator reads base facts from two sources, combined via `MultiEDB`:

### Fact Store

For present-time queries, the SQL compiler reads from the `current_facts` table -- a materialized view of only currently-valid, non-retracted facts. For temporal queries (`--as-of-valid`, `--as-of-tx`), it reads from the full `facts` table with appropriate temporal filters.

### Catalog (`CatalogEDB`)

Exposes `catalog_entries` as a `catalog_entry` relation with these fields:

| Field | Source |
|-------|--------|
| `id` | Entry ID |
| `schema` | CUE schema name |
| `origin` | Data origin / workspace |
| `format` | File format |
| `entry_type` | import, artifact, observe, manifest |
| `definition` | Definition name (if applicable) |
| `status` | Convergence status |
| `resource_id` | Stable resource identity |

Rules can join across both sources -- e.g., matching observations against catalog entries.

## Performance

Rules compile to SQL, so SQLite's query planner handles join ordering and index selection. The `current_facts` table is indexed on `relation` for fast base-case lookups. Recursive evaluation uses temp tables with primary key dedup, avoiding redundant re-derivation.

The safety limit for recursive fixpoint is 100 iterations. For typical workloads (hundreds of rules, thousands of facts), evaluation completes in milliseconds.
