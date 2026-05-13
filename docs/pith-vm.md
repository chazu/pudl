# Pith VM

Pith is a concatenative (stack-based) virtual machine embedded in pudl
and mu. Programs are JSON arrays of words. The VM interprets them
against a stack, dispatching each word to a registered handler.

pudl and mu share the same VM and data model (JSON on the stack) but
register different driver words. pudl registers read-only knowledge
operations; mu registers effectful execution words. An agent that
learns to write programs for pudl queries can immediately write
programs for mu actions.

The pith module lives at `~/dev/go/pith/` as a standalone Go module
with no dependencies beyond stdlib. Both pudl and mu import it.

## Language Basics

Programs are JSON arrays. Each element is one of:

| Type | Example | Behavior |
|------|---------|----------|
| String (word) | `"dup"` | Dispatch as word |
| String (`'` prefix) | `"'running"` | Push as string literal (without the `'`) |
| Number | `42`, `3.14` | Push as literal |
| Boolean | `true` | Push as literal |
| Null | `null` | Push as literal |
| Array | `["dup", "len"]` | Push as quotation (deferred program) |
| Object | `{"key": "val"}` | Push as data object |

Words execute immediately. Quotations are pushed onto the stack for
later execution by combinators like `apply`, `map`, or `if`.

### String Literals

Bare strings dispatch as words. To push a string value onto the stack,
prefix it with `'`:

```json
["'hello"]
```

This pushes the string `"hello"`. Without the prefix, `"hello"` would
be treated as a word lookup and fail with "unknown word: hello".

### Field References

Dotted strings like `"input.host"` resolve against named context maps
registered via `SetContext`. If the prefix matches a registered
context, the value is looked up and pushed. Otherwise the string is
treated as a word.

```bash
pudl exec --context host=example.com '["ctx.host"]'
# pushes "example.com"
```

## Vocabulary Reference

### Tier 1: Stack Primitives

```
dup    ( a -- a a )           Duplicate top
drop   ( a -- )               Discard top
swap   ( a b -- b a )         Swap top two
over   ( a b -- a b a )       Copy second to top
nip    ( a b -- b )           Discard second
rot    ( a b c -- b c a )     Rotate third to top
tuck   ( a b -- b a b )       Copy top under second
2dup   ( a b -- a b a b )     Duplicate top two
2drop  ( a b -- )             Discard top two
```

### Tier 2: Combinators

**Quotation combinators:**

```
apply   ( [q] -- ... )              Execute quotation
dip     ( a [q] -- ...q a )         Hide top, run quotation, restore
keep    ( a [q] -- ...q a )         Run quotation with a, keep a
bi      ( a [p] [q] -- p(a) q(a) )  Two ops on one value
bi*     ( a b [p] [q] -- p(a) q(b) ) One op each on two values
bi@     ( a b [q] -- q(a) q(b) )    Same op on two values
```

**Sequence combinators:**

```
each      ( seq [q] -- ... )         Apply to each element (no collect)
map       ( seq [q] -- seq' )        Transform each, collect results
filter    ( seq [q] -- seq' )        Keep elements where quotation is truthy
reduce    ( seq init [q] -- result ) Fold with accumulator
any?      ( seq [q] -- bool )        True if any element passes
all?      ( seq [q] -- bool )        True if all elements pass
group-by  ( seq [q] -- map )         Group by quotation result as key
flatten   ( seq -- seq' )            One-level array flattening
```

**Conditional:**

```
if      ( bool [then] [else] -- ... )  Branch on truthiness
when    ( bool [then] -- ... )         Execute if truthy
unless  ( bool [else] -- ... )         Execute if falsy
```

Falsy values: `false`, `nil`, `0`, `0.0`, `""`. Everything else is truthy.

### Tier 3: Data Words

**Object access:**

```
get     ( obj key -- value )       Get field (nil if missing)
set     ( obj key value -- obj' )  Set field (returns new object)
has?    ( obj key -- bool )        Field exists?
keys    ( obj -- [keys] )          All keys (sorted)
values  ( obj -- [values] )        All values (key-sorted)
path    ( obj "a.b.c" -- value )   Nested dotted-path access
```

**Object transformation:**

```
pick    ( obj [keys] -- obj' )     Keep only these fields
omit    ( obj [keys] -- obj' )     Remove these fields
merge   ( obj1 obj2 -- obj3 )      Shallow merge (obj2 wins)
```

**Comparison:**

```
eq      ( a b -- bool )    Equal (cross-type numeric: 1 == 1.0)
neq     ( a b -- bool )    Not equal
lt      ( a b -- bool )    Less than (numeric)
gt      ( a b -- bool )    Greater than (numeric)
lte     ( a b -- bool )    Less than or equal (numeric)
gte     ( a b -- bool )    Greater than or equal (numeric)
```

**Logic:**

```
and     ( a b -- bool )    Both truthy
or      ( a b -- bool )    Either truthy
not     ( a -- bool )      Negate truthiness
null?   ( a -- bool )      Is nil?
```

**Arithmetic:**

```
add     ( a b -- a+b )     Addition
sub     ( a b -- a-b )     Subtraction
mul     ( a b -- a*b )     Multiplication
div     ( a b -- a/b )     Division (errors on zero)
mod     ( a b -- a%b )     Modulo (errors on zero)
```

All arithmetic words coerce operands to float64. Mixed int/float
inputs work transparently.

**String and collection:**

```
concat  ( a b -- ab )      String or array concatenation
len     ( seq -- n )        Length of string, array, or map
split   ( str delim -- [parts] )  Split string by delimiter
```

### Tier 4: Driver Words

Driver words are registered by consumers and follow the naming
convention `namespace/verb`. pudl registers read-only words; mu
registers effectful words. The same program syntax works in both
contexts — only the available vocabulary differs.

## pudl Driver Words

These words are available in `pudl exec` and when mu imports pudl's
driver adapters.

### catalog/*

```
catalog/query   ( filters -- [entries] )   Query catalog entries
catalog/get     ( id -- entry )            Get entry by proquint or hex ID
catalog/count   ( filters -- n )           Count matching entries
```

The `filters` map accepts the same fields as `pudl list` flags:
`schema`, `origin`, `format`, `collections_only`, `items_only`.

### fact/*

```
fact/query    ( pattern -- [facts] )   Query facts
fact/assert   ( subj pred obj -- )     Assert a new fact
fact/retract  ( id -- )                Retract a fact by ID
```

### schema/*

```
schema/list    ( -- schemas )           List all schemas by package
schema/match   ( data -- schema_name )  Best matching schema (nil if none)
schema/infer   ( data -- result )       Full inference result map
```

`schema/match` returns the schema name string if confidence >= 0.2,
nil otherwise. `schema/infer` returns the full result with `schema`,
`confidence`, `reason`, `matched_at`, and `cascade_path` fields.

Both require a schema inferrer — available when schemas are loaded
from the workspace or global schema directory.

### drift/*

```
drift/diff   ( declared live -- [diffs] )   Field-level diff
```

Each diff in the result array has:
- `path` — dot-notation field path
- `type` — `"changed"`, `"added"`, or `"removed"`
- `declared` — value from declared map (nil for added)
- `live` — value from live map (nil for removed)

## pudl exec

Run pith programs against the pudl data lake from the command line.

```bash
pudl exec '<json-program>'
pudl exec -f program.json
echo '<json-program>' | pudl exec -
pudl exec -f program.json --trace
pudl exec --context schema=aws.#EC2Instance -f query.json
pudl exec --json '[{}, "catalog/count"]'
```

### Flags

| Flag | Description |
|------|-------------|
| `-f, --file` | Load program from a JSON file |
| `--trace` | Print stack state after each op to stderr |
| `--context key=value` | Set context values (repeatable). Available as `ctx.key` field refs. Values parsed as JSON when possible. |
| `--json` | Pretty-print result as indented JSON |

### Input Methods

Programs can be provided three ways:

1. **Inline argument** — shell quoting required for complex programs
2. **File** (`-f`) — avoids quoting issues
3. **Stdin** — pipe or heredoc, use `-` or just pipe

```bash
# Heredoc for readable multi-line programs
pudl exec - <<'EOF'
[{}, "catalog/query", "len"]
EOF

# Pipe from another command
cat program.json | pudl exec
```

### Trace Mode

`--trace` prints the stack state after each word to stderr:

```bash
$ pudl exec --trace '[3, 4, "add", 2, "mul"]'
  3            [ 3 ]
  4            [ 3 4 ]
  add          [ 7 ]
  2            [ 7 2 ]
  mul          [ 14 ]
14
```

Errors include the op index for debugging:

```
execution error: op 2 (get): expected map[string]any, got int
```

## Examples

### Count catalog entries by schema

```bash
pudl exec '[ {"schema": "aws.#EC2Instance"}, "catalog/count" ]'
```

### List schema package names

```bash
pudl exec --json '[ "schema/list", "keys" ]'
```

### Fleet summary — count and group by status

```bash
pudl exec --json -f - <<'EOF'
[
  {"schema": "aws.#EC2Instance"}, "catalog/query",
  "dup", "len",
  "swap",
  ["'status", "get"], "group-by"
]
EOF
```

Result stack: `[6, {"running": [...], "stopped": [...]}]`

### Filter running instances, extract IDs

```bash
pudl exec --json '[
  {"schema": "aws.#EC2Instance"}, "catalog/query",
  ["'status", "get", "'running", "eq"], "filter",
  ["'id", "get"], "map"
]'
```

### Diff two states for drift

```bash
pudl exec --json '[
  {"host": "web-1", "port": 8080, "replicas": 3},
  {"host": "web-1", "port": 9090, "zone": "us-east-1"},
  "drift/diff"
]'
```

### Arithmetic with catalog data

```bash
pudl exec '[
  {}, "catalog/count",
  100, "div"
]'
```

### Context-driven query

```bash
pudl exec --context schema=aws.#EC2Instance --context origin=aws \
  '[ {"schema": "ctx.schema", "origin": "ctx.origin"}, "catalog/count" ]'
```

Note: context values in filter maps must use `ctx.*` field refs
inside the program — the `--context` flag sets them as VM context,
not as direct substitutions.

## CUE Schema

pith ships a CUE package for validating programs at definition time.
Import it in mu or pudl CUE configs:

```cue
import "github.com/chazu/pith"

myMethod: {
    body: pith.#Program & [
        "dup", ["'name", "get"], "keep",
    ]
}
```

`#Program` validates each operation against the full vocabulary.
Unknown words and malformed ops produce CUE unification errors before
the interpreter runs.

## Architecture

```
pith (standalone Go module — no external deps)
  ├── vm.go         VM struct, stack, dispatch loop
  ├── builtins.go   Tier 1 + Tier 2 words
  ├── data.go       Tier 3 data + arithmetic words
  ├── cue.go        CUE program extraction
  └── cue.cue       #Program/#Op schemas

pudl (imports pith)
  └── internal/pithdriver/
      ├── register.go       Register(vm, db, mgr, inferrer)
      ├── catalog.go        catalog/* words
      ├── facts.go          fact/* words
      ├── schema.go         schema/list
      ├── schema_infer.go   schema/match, schema/infer
      ├── drift.go          drift/diff
      └── convert.go        JSON round-trip helpers

mu (imports pith)
  └── internal/pithvm/
      └── register.go       Phase-scoped drivers (plan/transform/exec)
```

Both pudl and mu import pith directly. Neither depends on the other.
