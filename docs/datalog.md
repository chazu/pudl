# Datalog Evaluator

PUDL includes a Datalog evaluator that derives new facts from existing ones using inference rules. It reads base facts (EDB) from both the [bitemporal fact store](facts.md) and the catalog, evaluates rules to a fixed point, and returns derived facts (IDB).

## How It Works

The evaluator uses **semi-naive bottom-up evaluation**:

1. Load all base facts from EDB sources (fact store + catalog)
2. For each rule, find all variable substitutions that satisfy the body
3. Apply the substitution to the head to produce a derived fact
4. If new facts were derived, repeat from step 2
5. Stop when no new facts are produced (fixed point)

An in-memory hash index (relation + arg key + arg value) accelerates joins. When a body atom has a ground term or an already-bound variable, the evaluator does an O(1) index lookup instead of scanning all tuples.

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

The evaluator loads rules from both global and repo-scoped directories, evaluates to fixed point, then filters results by the requested relation and constraints.

| Flag | Description |
|------|-------------|
| `-f, --rule-file` | Load additional rules from a CUE file |
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

### Fact Store (`FactsEDB`)

Reads from the `facts` table using `AsOfNow` temporal mode (current valid, not retracted). Each fact's JSON `args` are parsed into a tuple. The relation name comes from the `relation` column.

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

The evaluator uses hash indexes for joins. For each relation, tuples are indexed by every arg key + value combination. When matching a body atom with a bound variable or ground term, the evaluator does an O(1) hash lookup instead of scanning all tuples.

For typical workloads (hundreds of rules, thousands of facts), evaluation completes in milliseconds. The safety limit is 100 iterations before the evaluator stops (configurable).

Transitive closure of a 10-node graph (55 derived paths) runs in under 1ms.
