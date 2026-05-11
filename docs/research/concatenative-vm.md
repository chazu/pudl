# Concatenative VM for pudl and mu

Research document exploring an embedded stack-based (concatenative) language
for defining methods on models in CUE, interpreted by a small Go runtime.

## Motivation

Both pudl and mu need a way for agents to author small programs:
- **pudl**: data transforms, computed views, enrichment pipelines over catalog data
- **mu**: lightweight plugin logic, API call orchestration, inter-target glue

Currently, pudl has no execution capability (by design: "pudl knows, mu acts"),
and mu requires full plugin binaries (Go/Python/etc.) for any logic beyond
shell commands. There's a gap between "shell one-liner" and "compile a binary"
that neither tool serves well.

A concatenative (stack-based) language embedded in CUE fills this gap.
Programs are stored as CUE data — typed lists of words — validated at
definition time, and interpreted by a ~150-line Go VM that both tools share.

### Why Concatenative Programs Are Agent-Friendly

- **No variables, no scoping, no binding** — just word lists. Nothing to
  shadow, capture, or close over.
- **Composition by juxtaposition** — appending words to a list composes
  behavior. No syntax to get wrong.
- **Programs are data** — a CUE list `[...]` is both the program
  representation and its quotation syntax.
- **Vocabulary is introspectable** — agents query the CUE definition to
  discover available words before generating programs.
- **CUE validates vocabulary** — typos and unknown words caught at eval time,
  before the interpreter runs.

An LLM generates `["input.host", "dns/lookup", ["ping"], "each", "filter"]`
more reliably than correct Go with error handling. The vocabulary constraint
means the failure mode is "wrong word order" not "syntax error."

## Language Design

### Why Plain Concatenative (Not Retro, Not Forsp)

Three Forth variants were evaluated:

**Retro (sigils/prefixes):** Uses `#42` for numbers, `'word` for pointers,
`$str` for strings. Sigils solve token-level type disambiguation in a text
stream. But CUE already distinguishes strings from numbers from nested lists
— sigils are redundant when the program is typed structured data.

**Forsp (call-by-push-value):** Distinguishes values (`$x` pushed) from
computations (bare `x` called). Eliminates explicit quotation syntax in text.
But in a CUE list, the `$`-prefix encoding loses Forsp's syntactic elegance.
The interpreter needs environment frames and closure allocation (~300+ lines
vs ~150). And closures/variable binding are exactly the scoping complexity
we want to avoid for agent authoring.

**Plain concatenative (Joy/Factor lineage):** Words are words. Nested CUE
lists are quotations. No variables. Combinators handle control flow. Factor-
style vocabularies for namespacing. Smallest interpreter surface. Natural fit
for CUE embedding because `[...]` is already a list value — the
representation *is* the semantics.

### Program Representation in CUE

Programs are CUE lists. Nested lists are quotations (deferred execution).
Strings are word references. Numbers and bools are literals pushed to stack.

```cue
#Method: {
    description: string
    input:       #Schema
    output:      #Schema
    body:        [...#Op]
}

methods: listVms: #Method & {
    description: "List all VMs on a Proxmox node"
    input: { host: string, ticket: string }
    output: { vms: [...#VM] }
    body: [
        "input.host", "input.ticket",
        "proxmox/auth",
        "nodes", "proxmox/get",
        "flatten",
        ["dup", "node", "get", "qemu", "proxmox/get"],
        "each",
        "concat",
    ]
}
```

### CUE Vocabulary Validation

Each tier of the vocabulary maps to a CUE union. CUE catches unknown words
at definition time:

```cue
#StackOp:    "dup" | "drop" | "swap" | "over" | "nip" | "rot" | "tuck"
#CombOp:     "apply" | "dip" | "keep" | "bi" | "bi*" | "bi@"
#SeqOp:      "each" | "map" | "filter" | "reduce" | "any?" | "all?"
#CondOp:     "if" | "when" | "unless"
#DataOp:     "get" | "set" | "has?" | "keys" | "values" | "path" |
             "pick" | "omit" | "merge"
#CompareOp:  "eq" | "neq" | "lt" | "gt" | "lte" | "gte"
#LogicOp:    "and" | "or" | "not" | "null?"
#StringOp:   "concat" | "len" | "split"

#DriverOp:   =~"^[a-z]+/[a-z_]+$"
#FieldRef:   =~"^(input|output|data)\\."
#StringLit:  =~"^'."

#Op: #StackOp | #CombOp | #SeqOp | #CondOp |
     #DataOp | #CompareOp | #LogicOp | #StringOp |
     #DriverOp | #FieldRef | #StringLit |
     number | bool | string |
     [...#Op]
```

## Combinator Vocabulary Design

### Tier 1: Stack Primitives

The irreducible core for data manipulation on the stack:

```
dup    ( a -- a a )
drop   ( a -- )
swap   ( a b -- b a )
over   ( a b -- a b a )
nip    ( a b -- b )
rot    ( a b c -- b c a )
tuck   ( a b -- b a b )
2dup   ( a b -- a b a b )
2drop  ( a b -- )
```

An agent needs ~5 of these (`dup`, `drop`, `swap`, `over`, `rot`) to be
productive. The rest are convenience.

### Tier 2: Quotation Combinators

Quotation combinators eliminate stack shuffling. This is the most important
design layer — excessive shuffling (`swap rot over nip tuck rot`) is the
concatenative equivalent of spaghetti code.

**Design principle: if a program needs more than 2 consecutive stack shuffles,
the combinator vocabulary is missing a word.**

**Execution:**

```
apply   ( [q] -- ... )              — execute quotation
```

**Preserving combinators** (the workhorses):

```
dip     ( a [q] -- ...q a )         — hide TOS, run quotation, restore
keep    ( a [q] -- ...q a )         — run quotation on a, keep a after
```

`dip` and `keep` solve 80% of "I need this value later but want to do
something else first" without named variables.

```cue
// Get VM name but keep the VM object on stack
body: ["vm", ["name", "get"], "keep"]
// stack after: "my-vm"  {vm-object}
```

**Cleaving combinators** (apply multiple quotations to same value):

```
bi      ( a [p] [q] -- p(a) q(a) )  — two ops on one value
bi*     ( a b [p] [q] -- p(a) q(b) )— one op each on two values
bi@     ( a b [q] -- q(a) q(b) )    — same op on two values
```

These replace error-prone `dup [do-x] dip [do-y]` patterns. `bi` is
declarative — "apply both of these to that":

```cue
// Extract name and state from VM object
body: ["vm", ["name", "get"], ["state", "get"], "bi"]
// stack after: "my-vm"  "running"
```

**Sequence combinators:**

```
each    ( seq [q] -- ... )           — apply to each element
map     ( seq [q] -- seq' )          — transform each, collect
filter  ( seq [q] -- seq' )          — keep where quotation truthy
reduce  ( seq init [q] -- result )   — fold with accumulator
any?    ( seq [q] -- bool )          — short-circuit exists
all?    ( seq [q] -- bool )          — short-circuit forall
```

```cue
// Get names of all running VMs
body: [
    "proxmox/list-vms",
    ["state", "get", "running", "eq"], "filter",
    ["name", "get"], "map",
]
```

**Conditional:**

```
if      ( bool [then] [else] -- ... )
when    ( bool [then] -- ... )
unless  ( bool [else] -- ... )
```

### Tier 3: Data Words

JSON objects and arrays are the primary data types. These words operate on
them idiomatically:

**Object access:**

```
get     ( obj key -- value )         — get field
set     ( obj key value -- obj' )    — set field (returns new obj)
has?    ( obj key -- bool )          — field exists?
keys    ( obj -- [keys] )            — all keys
values  ( obj -- [values] )          — all values
path    ( obj "a.b.c" -- value )     — nested dotted-path access
```

**Object transformation:**

```
pick    ( obj [keys] -- obj' )       — keep only these fields
omit    ( obj [keys] -- obj' )       — remove these fields
merge   ( obj1 obj2 -- obj3 )        — shallow merge (obj2 wins)
```

**Comparison and logic:**

```
eq neq lt gt lte gte                 — comparisons
and or not                           — boolean
null?                                — null check
```

**String and numeric:**

```
concat  ( a b -- ab )                — string or array concat
len     ( seq -- n )                 — length
split   ( str delim -- [parts] )     — string split
```

### Tier 4: Driver Words (Namespaced)

Everything effectful or domain-specific lives in registered vocabularies.
Convention: `namespace/verb`. CUE validates with `=~"^[a-z]+/[a-z_]+$"`.

The vocabulary a program can use depends on its execution context (pudl vs
mu). The interpreter is the same; the registered driver words differ.

### Minimum Viable Vocabulary

Ship first with ~25 words:

```
Stack:   dup drop swap
Comb:    apply dip keep bi each map filter if
Data:    get set has? keys merge path
Logic:   eq neq not null?
String:  concat len
```

Plus whatever driver words the first use case needs. Add words only when a
real program requires them. Factor started with ~30 core words and grew to
thousands — the core 30 still handle 90% of code.

## Integration with pudl

pudl's execution boundary: **read-only knowledge operations**. The VM
operates on catalog data, schemas, and facts. It does not mutate external
systems. This preserves "pudl knows, mu acts."

### pudl Driver Words

```
catalog/query   ( filters -- [entries] )    — query catalog
catalog/get     ( id -- entry )             — get entry by ID
catalog/count   ( filters -- n )            — count matching
schema/list     ( -- [schemas] )            — list schemas
schema/match    ( data -- schema )          — match data to schema
schema/infer    ( data -- schema )          — infer schema
fact/assert     ( subj pred obj -- )        — assert fact
fact/query      ( pattern -- [facts] )      — query facts
fact/retract    ( id -- )                   — retract fact
```

### Use Cases in pudl

**Computed views:** Define derived data as methods on schemas.

```cue
methods: summary: #Method & {
    description: "Summarize EC2 fleet"
    input: {}
    output: { total: int, by_state: _ }
    body: [
        {"schema": "aws.#EC2Instance"}, "catalog/query",
        "dup", "len", "output.total", "set",
        ["state", "get"], "map",
        "group-by", "output.by_state", "set",
    ]
}
```

**Enrichment pipelines:** Transform imported data at query time.

**Cross-resource correlation:** Link resources across providers using
stack programs that join on shared fields.

## Integration with mu

mu gains three integration points, from shallow to deep.

### Level 1: Actions as Stack Programs

mu actions currently have a `command` field (shell string). Add an
alternative `body` field (stack program list):

```cue
// Current: shell command
action: {
    command: "curl -s https://api.example.com/vms | jq '.items'"
    inputs: [...]
    outputs: [...]
}

// New: stack program
action: {
    body: [
        "https://api.example.com/vms", "http/get",
        "items", "get",
    ]
    inputs: [...]
    outputs: [...]
}
```

Executor checks: `body` present → interpret with VM. `command` present →
shell exec in sandbox. Caching works identically —
`hash(body + input_digests)` instead of `hash(command + input_digests)`.

**Benefit:** Hermetic, cacheable actions without shelling out. No `curl | jq`
pipelines that break on edge cases. The VM handles JSON natively.

### Level 2: Plan Methods as Stack Programs

Currently, plugins implement `plan` as compiled code in a binary. Stack
programs can replace simple plugins entirely:

```cue
target: {
    name: "//infra/dns-record"
    plan: [
        "config", "get",
        "dup", "record_type", "get",
        "A", "eq",
        [
            ["host", "get"], ["ip", "get"], "dns/create-a",
        ],
        [
            ["host", "get"], ["target", "get"], "dns/create-cname",
        ],
        "if",
        "action/emit",
    ]
}
```

The coordinator sees `plan` field → interprets it → collects emitted
actions → merges into DAG. No plugin process spawned.

The key word is `action/emit` — a driver word that *declares* an action
into the DAG rather than executing one. The plan program runs at planning
time; the emitted actions run later during execution. This preserves mu's
plan/execute split.

### Level 3: Orchestration Glue Between Targets

Stack programs as inter-target transforms — glue logic that transforms
output of target A before feeding to target B:

```cue
target: {
    name: "//deploy/config"
    depends: ["//infra/vpc", "//infra/db"]
    transform: [
        "//infra/vpc", "target/output", "vpc_id", "get",
        "//infra/db", "target/output", "endpoint", "get",
        "connection_string", "format/db-url",
        "config", "swap", "db_url", "set",
    ]
}
```

### mu Driver Words

```
// HTTP
http/get      ( url -- response )
http/post     ( url body -- response )
http/put      ( url body -- response )
http/delete   ( url -- response )
http/header   ( key value -- )            // set for next request

// Process execution
exec/run      ( [args] -- stdout )
exec/shell    ( cmd -- stdout )

// CAS (content-addressed store)
cas/store     ( data -- digest )
cas/fetch     ( digest -- data )

// Secrets (integrates with mu's sealed inputs)
secret/resolve ( ref -- value )           // "pass:deploy/token" → value

// DAG construction (plan-time only)
action/emit    ( spec -- )                // emit ActionSpec into DAG
action/depends ( target -- )              // declare dependency

// Target cross-references
target/output  ( name -- data )           // read dependency output
target/config  ( -- config )              // current target config
```

### What Stays as Plugin Binaries

Stack programs replace simple plugins. Complex plugins stay as binaries:

| Use Case | Stack Program | Plugin Binary |
|----------|:---:|:---:|
| API call + transform | yes | |
| File write with permissions | yes | |
| Config templating | yes | |
| Go compilation | | yes |
| Docker build | | yes |
| Terraform apply | | yes |
| Subprocess streaming | | yes |
| Persistent state across actions | | yes |

Rule of thumb: if the logic is "call API, transform data, emit result" —
stack program. If it needs a toolchain, long-running process, or complex
I/O — plugin binary.

### mu Coordinator Changes

mu's coordinator runs a 5-phase pipeline: build toolchains → start
plugins → plan targets → merge DAGs → execute. pith integrates at
phases 3 (plan) and 5 (execute).

**Phase 3 — Plan targets:** Currently, the coordinator sends each target
to its plugin's `plan` method, which returns an ActionSpec array. With
pith, the coordinator first checks if the target has a `plan` field
(CUE list). If so, it creates a pith VM, registers mu driver words,
pushes the target config onto the stack, and runs the program. The
`action/emit` word collects ActionSpecs into a buffer. After the program
completes, the coordinator takes the collected specs and merges them
into the DAG — same as it would with plugin-returned specs.

```go
// In coordinator plan phase:
if planProgram := target.Plan; planProgram != nil {
    vm := pith.New(ctx)
    registerMuDrivers(vm, coordinator, dag)
    vm.Push(target.Config)
    if err := vm.Run(planProgram); err != nil {
        return fmt.Errorf("target %s plan: %w", target.Name, err)
    }
    actions := drainEmittedActions(vm)
    dag.MergeActions(target.Name, actions)
} else {
    // existing path: dispatch to plugin
    plugin.Plan(target)
}
```

**Phase 5 — Execute actions:** The DAG executor currently runs each
action's `command` in a sandbox. With pith, actions can have a `body`
field instead. The executor checks which field is present:

```go
func (e *Executor) runAction(action Action) error {
    if action.Body != nil {
        vm := pith.New(e.ctx)
        registerMuDrivers(vm, e.coordinator, nil) // no DAG mutation at exec time
        for _, input := range action.ResolvedInputs {
            vm.Push(input)
        }
        if err := vm.Run(action.Body); err != nil {
            return err
        }
        result, _ := vm.Result()
        return e.storeOutput(action, result)
    }
    return e.sandboxExec(action)
}
```

**Driver word context:** mu driver words need access to coordinator
state (CAS, secrets, dependency outputs). This is injected via closures
at registration time — the VM itself stays generic:

```go
func registerMuDrivers(vm *pith.VM, coord *Coordinator, dag *DAG) {
    // HTTP vocabulary
    vm.RegisterDriver("http", map[string]pith.Word{
        "get":    makeHTTPGet(coord.HTTPClient),
        "post":   makeHTTPPost(coord.HTTPClient),
        "put":    makeHTTPPut(coord.HTTPClient),
        "delete": makeHTTPDelete(coord.HTTPClient),
        "header": makeHTTPHeader(coord.HTTPClient),
    })

    // CAS vocabulary
    vm.RegisterDriver("cas", map[string]pith.Word{
        "store": func(vm *pith.VM) error {
            data, _ := vm.Pop()
            digest, err := coord.CAS.Store(vm.Context(), data)
            if err != nil { return err }
            vm.Push(digest)
            return nil
        },
        "fetch": func(vm *pith.VM) error {
            digest, _ := vm.Pop()
            data, err := coord.CAS.Fetch(vm.Context(), digest.(string))
            if err != nil { return err }
            vm.Push(data)
            return nil
        },
    })

    // Secret vocabulary (wraps mu's sealed input resolution)
    vm.RegisterDriver("secret", map[string]pith.Word{
        "resolve": func(vm *pith.VM) error {
            ref, _ := vm.Pop()
            val, err := coord.ResolveSecret(vm.Context(), ref.(string))
            if err != nil { return err }
            vm.Push(val)
            return nil
        },
    })

    // Target cross-references
    vm.RegisterDriver("target", map[string]pith.Word{
        "output": func(vm *pith.VM) error {
            name, _ := vm.Pop()
            data, err := coord.GetTargetOutput(name.(string))
            if err != nil { return err }
            vm.Push(data)
            return nil
        },
        "config": func(vm *pith.VM) error {
            vm.Push(coord.CurrentTarget().Config)
            return nil
        },
    })

    // DAG construction (plan-time only — nil dag at exec time)
    if dag != nil {
        vm.RegisterDriver("action", map[string]pith.Word{
            "emit": func(vm *pith.VM) error {
                spec, _ := vm.Pop()
                dag.AddAction(spec.(map[string]any))
                return nil
            },
            "depends": func(vm *pith.VM) error {
                target, _ := vm.Pop()
                dag.AddDependency(target.(string))
                return nil
            },
        })
    }
}
```

### CUE Config Schema Changes

mu's `mu.cue` target schema gains optional `plan` and action `body`
fields alongside existing `toolchain`/`command`:

```cue
import "github.com/chazu/pith"

#Target: {
    name:       string & =~"^//[a-z]"
    toolchain?: string
    sources?:   [...string]
    config?:    _
    depends?:   [...string]

    // Inline plan program — alternative to plugin-based planning.
    // If present, coordinator interprets this instead of calling a plugin.
    plan?: pith.#Program

    // Inter-target data transform — runs after dependencies complete,
    // before this target's actions execute.
    transform?: pith.#Program
}

#Action: {
    // Shell command (existing)
    command?: string

    // Stack program (new — mutually exclusive with command)
    body?: pith.#Program

    inputs?:  [...#ArtifactRef]
    outputs?: [...#ArtifactRef]

    // CUE enforces mutual exclusivity
    _check: true & (
        (command != _|_ & body == _|_) |
        (command == _|_ & body != _|_)
    )
}
```

CUE validates programs against `pith.#Program` at config load time.

### Integration Design Decisions

Resolved during Q1-Q7 design review (2026-05-11):

**Phase-scoped driver registration.** Three execution phases, each gets
a different driver word set. Words unavailable in a phase produce
"unknown word" errors — no silent misuse.

| Word | Plan | Transform | Execute |
|---|:---:|:---:|:---:|
| `action/emit` | ✓ | | |
| `action/depends` | ✓ | | |
| `target/config` | ✓ | ✓ | ✓ |
| `target/output` | | ✓ | ✓ |
| `http/*` | | | ✓ |
| `exec/*` | | | ✓ |
| `cas/*` | | | ✓ |
| `secret/resolve` | | | ✓ |
| `catalog/*` (pudl) | ✓ | ✓ | ✓ |
| `fact/*` (pudl) | ✓ | ✓ | ✓ |
| `schema/*` (pudl) | ✓ | ✓ | ✓ |

Implementation: three registration functions (`registerPlanDrivers`,
`registerTransformDrivers`, `registerExecDrivers`) that each call
`vm.RegisterDriver` with the appropriate subset.

**`target/output` timing.** Only available in transform and execute
phases. Transform bodies run after dependencies complete but before
own actions execute. Plan programs cannot read dependency outputs —
they only see own config via `target/config`. This avoids stale-cache
reads and keeps plan programs hermetic.

Execution order within mu's pipeline:
```
dependencies execute → outputs stored in CAS
    ↓
transform body runs → reads dependency outputs via target/output
    ↓
own actions execute (body or command)
```

**`action/emit` semantics.** Closure-based buffer. Plan-time VM
captures emitted ActionSpecs in a slice via closure. After `vm.Run`
completes, coordinator drains the buffer and merges specs into the
DAG. Multiple emits per plan program are supported. `action/emit`
returns nothing (stack effect: `( spec -- )`).

**Error propagation.** pith errors map directly to mu's existing
failure model. `vm.Run` returns `error` → mu wraps with context:
- Plan phase: `"target //infra/dns: plan: op 3 (get): ..."` → target
  skipped, no actions emitted.
- Execute phase: `"action create-record: op 2 (http/post): ..."` →
  action failed, transitive dependents cancelled, independent
  subgraphs continue.

No new error types or handling needed.

**Secrets in traces.** Caller responsibility. Trace mode is opt-in
(`pith.NewWithTrace`). mu's executor does not enable trace in
production. If a developer enables trace on a program using
`secret/resolve`, that's an intentional debugging choice. No
redaction mechanism in v1 — add `SecretValue` wrapper type later
if it becomes a real problem.
Unknown words, malformed ops, and type errors are caught before the
coordinator starts — same as existing CUE validation for target configs.

### Caching Mechanics

mu's CAS cache keys actions by `hash(command + input_digests)`. For
pith actions, the cache key becomes `hash(canonical(body) + input_digests)`
where `canonical(body)` is the JSON serialization of the `[]any` program.

This works because pith programs are deterministic — same program + same
inputs = same outputs. The program itself is pure data (a list), so
serialization is trivial. No function pointers or closures in the cache
key.

Edge case: driver words with side effects (HTTP calls, secret resolution)
produce non-deterministic results. mu already handles this via the
`cacheable` flag on actions — same flag applies to pith actions. An
action with `body` that calls `http/get` should be marked non-cacheable,
just like a shell action that calls `curl`.

### The Observe Loop (mu → pudl)

mu's `observe` command queries live state via plugins. With pith, observe
programs can run inline:

```cue
target: {
    name: "//infra/dns"
    observe: [
        "dns.example.com", "dns/lookup",
        ["type", "get", "A", "eq"], "filter",
        ["value", "get"], "map",
    ]
}
```

The coordinator interprets the `observe` program, serializes the result
as JSON, and emits it on stdout (same as plugin observe output). pudl
consumes this via `mu observe --json //target | pudl import --stdin`.

This closes the full loop:

```
pudl definition (desired state)
  → pudl drift check (compare with catalog)
  → pudl export-actions (emit mu.json with pith programs)
  → mu build (coordinator interprets plan programs, executes action programs)
  → mu observe (coordinator interprets observe programs)
  → pudl import (catalog updated with new state)
  → pudl drift check (verify convergence)
```

At every stage, the programs are CUE lists validated by `pith.#Program`.
The agent authors them; CUE validates them; pith interprets them. No
compiled plugins needed for the common case.

### Plugin Protocol Coexistence

pith does not replace the NDJSON plugin protocol. They coexist:

| Mechanism | When to use | How it works |
|-----------|-------------|--------------|
| NDJSON plugin | Heavy lifting: compilation, complex I/O, streaming | Separate process, stdin/stdout protocol |
| pith `plan` | Simple planning: conditional action selection | Inline CUE program, coordinator interprets |
| pith `body` | Simple actions: API calls, data transforms | Inline CUE program, executor interprets |
| pith `observe` | Simple observation: API queries, status checks | Inline CUE program, coordinator interprets |
| pith `transform` | Glue logic: reshape data between targets | Inline CUE program, runs between deps and actions |

A single target can mix both: `toolchain: "go"` dispatches to the Go
plugin for compilation, while a `transform` pith program reshapes the
output before downstream targets consume it. The plugin handles the
heavy work; pith handles the data wiring.

Targets with `plan` fields skip plugin dispatch entirely — the
coordinator handles planning inline. This is the "lightweight plugin"
use case: instead of writing a Go binary that speaks NDJSON, you write
a CUE list of words.

## Module Architecture: pith

The VM lives in its own Go module — `pith` — at `~/dev/go/pith/`.
Same extraction pattern as `ewe` (CUE custom functions, `~/dev/go/ewe/`).
Both pudl and mu import pith; neither depends on the other.

### Why a Standalone Module (Not Internal to pudl)

The VM and ewe solve fundamentally different problems at different layers:

- **ewe**: single function calls embedded in CUE, resolved at CUE eval
  time by AST rewriting. Stateless — each call is independent. The output
  *is* a CUE value.
- **pith**: multi-step programs with a stack, control flow, and state
  threading. Runs *after* CUE evaluation. The CUE value *describes* the
  program; the Go interpreter *executes* it.

They are sequential in the pipeline, not nested:

```
CUE source
  → ewe rewrites function calls (AST level)
  → CUE evaluates (unification)
  → pith programs extracted as concrete CUE values
  → Go interpreter runs them (runtime level)
```

ewe could produce *inputs* to pith programs (computed args), but pith
itself is not a CUE function. They are peers — both extend what you can
do with CUE, but at different layers.

Three placement options were evaluated:

1. **Standalone module (`~/dev/go/pith/`)** — chosen. mu needs to import
   it. If it lives inside pudl (even as `pkg/`), mu depends on pudl's
   module — a coupling direction that doesn't exist today and shouldn't.
   ewe proved the pattern: extract shared capability into its own module.

2. **`pkg/forthvm/` inside pudl** — works short-term but mu can't import
   it without depending on pudl's module. Creates an unwanted dependency
   direction.

3. **`internal/` inside pudl** — too restrictive. Can't be imported by mu
   or anything else.

### Module Structure

```
pith/
├── go.mod           # standalone, only stdlib deps
├── vm.go            # VM struct, stack, dispatch loop (~100 lines)
├── word.go          # Word type, driver registration
├── builtins.go      # Core words: dup, swap, each, map, filter, bi...
├── data.go          # JSON data words: get, set, path, merge...
├── cue.go           # Extract programs from CUE values
└── pith_test.go
```

### Dependency Graph

```
pudl imports: ewe (custom CUE functions) + pith (VM for queries)
mu   imports: pith (VM for actions/plans)
```

No dependency between ewe and pith. No dependency between pudl and mu.
Clean diamond — each tool picks up the shared libraries it needs.

### Core API

```go
type VM struct {
    stack  []any
    words  map[string]Word
    ctx    context.Context
}

type Word func(vm *VM) error

func (vm *VM) RegisterDriver(prefix string, words map[string]Word) {
    for name, w := range words {
        vm.words[prefix+"/"+name] = w
    }
}
```

pudl registers read-only words (`catalog/query`, `schema/match`,
`fact/query`). mu registers effectful words (`http/get`, `exec/run`,
`action/emit`). Same VM, different capabilities based on context.

### The Emergent Property

When both tools share the VM and data model (JSON on stack):

```
pudl: "what VMs exist?"
  → catalog/query → filter → map → result

mu: "make this VM exist"
  → target/config → proxmox/create → action/emit

pudl: "did it drift?"
  → catalog/query → definition/get → diff → drift/report

mu: "fix the drift"
  → drift actions from pudl → proxmox/update → converge
```

Same language, same data model, different driver words. An agent that learns
to write programs for pudl queries can immediately write programs for mu
actions. The vocabulary is the only difference — and it's introspectable
from CUE.

This is the real payoff: not "a nicer way to write plugins" but a shared
computational medium between the knowledge layer and the execution layer
that an agent can operate fluently across both.

### Relationship to the Toolchain

```
ewe ─────────────── CUE eval time ─── AST rewriting, function resolution
                         │
                    CUE unification
                         │
pith ────────────── runtime ───────── program interpretation, driver dispatch
  ├── pudl context: read-only driver words (catalog, schema, fact)
  └── mu context:   effectful driver words (http, exec, cas, action)
```

## Implementation Status

pith VM is complete at `~/dev/go/pith/`. 97 tests passing. All four
tiers implemented:

- **Tier 1 (stack):** dup, drop, swap, over, nip, rot, tuck, 2dup, 2drop
- **Tier 2 (combinators):** apply, dip, keep, bi, bi*, bi@, each, map,
  filter, reduce, any?, all?, if, when, unless
- **Tier 3 (data):** get, set, has?, keys, values, path, pick, omit,
  merge, eq, neq, lt, gt, lte, gte, and, or, not, null?, concat, len, split
- **Tier 4 (drivers):** RegisterDriver API works, no drivers shipped
  (consumers register their own)
- **CUE integration:** ExtractProgram converts cue.Value → []any,
  cue.cue ships #Program/#Op schemas
- **Trace mode:** NewWithTrace prints stack after each word

Module structure matches the plan. Only stdlib deps in core; cue.go
imports cuelang.org/go for program extraction.

### Resolved Design Decisions

**Module placement:** Standalone at `~/dev/go/pith/`. Confirmed by ewe
precedent. mu imports pith directly; no pudl→mu coupling.

**action/emit pattern (mu):** Driver words close over coordinator state.
`action/emit` appends to an external `[]ActionSpec` via closure.
Coordinator drains buffer after `vm.Run` completes. No side-channel
in VM itself.

**CUE schema imports:** Consumers import `github.com/chazu/pith` for
`#Program` type. CUE validates programs at definition time.

## Open Questions

These must be resolved before pudl/mu integration:

### Q1: Field Reference Resolution ✅

**Resolved:** Option 2 — catch-all resolver in VM dispatch.

`VM.SetContext(name, data)` registers named context maps. Field refs like
`"input.host"` resolve via fallback in `Run`: split on `.`, check prefix
against registered contexts, walk nested maps. Missing keys push `nil`.
Traces show original ref name. No extraction rewriting, no explicit
`get`/`path` boilerplate.

```go
vm.SetContext("input", map[string]any{"host": "example.com"})
vm.Run([]any{"input.host"}) // pushes "example.com"
```

Implementation: `pith/vm.go` — `refs` field, `SetContext()`, `resolveFieldRef()`.
Tests: `pith/vm_test.go` — 7 cases (simple, nested, missing key, unknown
prefix, multiple contexts, mixed with words, trace output).

### Q2: Execution Entry Point in pudl ✅

**Resolved:** No pudl-side entry point for v1. mu imports pudl's driver
word adapters as a Go package and owns the trigger. pudl's role is
exposing catalog/fact/schema operations as importable driver words (Q3).

A `pudl exec` CLI command for testing/debugging methods will be added
later, informed by real usage patterns from mu integration.

### Q3: Driver Word ↔ pudl API Adapter Layer ✅

**Resolved:** `internal/pithdriver/` package with JSON round-trip conversion.

Type mapping uses generic `mapToStruct[T]` / `structToMap` helpers that
marshal through `encoding/json`. All pudl structs already have json tags.
Microsecond overhead, irrelevant for DB-bound operations.

Write words (`fact/assert`, `fact/retract`) included in pudl's driver —
facts are pudl's domain, mu just triggers them.

Implemented words:

| Word | Stack signature | Status |
|---|---|---|
| `catalog/query` | `( filters -- [entries] )` | ✅ |
| `catalog/get` | `( id -- entry )` | ✅ (tries proquint then raw ID) |
| `catalog/count` | `( filters -- n )` | ✅ |
| `fact/query` | `( pattern -- [facts] )` | ✅ |
| `fact/assert` | `( subj pred obj -- )` | ✅ |
| `fact/retract` | `( id -- )` | ✅ |
| `schema/list` | `( -- schemas )` | ✅ |
| `schema/match` | `( data -- schema )` | deferred (API doesn't exist) |
| `schema/infer` | `( data -- schema )` | deferred (API doesn't exist) |

Usage: `pithdriver.Register(vm, db, schemaMgr)` registers all three
namespaces. Callers pass nil for unused components.

### Q4: mu's Actual Architecture vs Research Assumptions ✅

**Resolved:** No mismatch. Research doc assumptions were correct.

mu has a Go coordinator (`cmd/mu/main.go`) that dispatches to plugins
via NDJSON over stdin/stdout. Babashka `.bb` scripts are one plugin
runtime, not an alternative architecture. Go coordinator resolves `bb`
binary as a toolchain artifact, spawns plugins as subprocesses.

Pipeline: Config (CUE) → Plan (plugins emit ActionSpecs via NDJSON) →
DAG construction → topological parallel execution → outputs to CAS.

pith integrates in-process: targets/actions with `body` field (CUE list)
run via pith VM instead of subprocess. No NDJSON overhead for simple
transform-and-emit logic. Coexists with `.bb` plugin dispatch.

### Q5: Missing Vocabulary ✅

**Resolved:** Two words implemented, two deferred.

- `group-by` ( seq [q] -- map ) — ✅ implemented. Quotation extracts key
  from each element, returns `map[string][]any`. Key is `fmt.Sprintf("%v", k)`.
- `flatten` ( seq -- seq' ) — ✅ implemented. One-level array flattening.
  Non-array elements pass through.
- `format/*` — deferred. Domain-specific driver words, mu's responsibility.
- `diff` — deferred. Useful for drift detection but no consumer yet.

**Discovered issue: no string literal syntax.** All strings in programs
dispatch as word lookups. `["state", "get"]` fails because `"state"` is
treated as a word, not a literal. This breaks every research doc example
that uses `get`/`set`/`path` with inline string keys. See Q7 below.

### Q6: Error Reporting for Agent-Authored Programs ✅

**Resolved:** Op index in all error messages + trace mode for step-by-step.

Errors now include position: `"op 3 (get): expected map[string]any, got int"`
and `"op 2: unknown word: staet"`. Index is zero-based, matches program
array position. Nested quotations (via apply/each/map/etc.) have their
own indices starting at 0 — the word name in the outer error provides
context for which combinator invoked the inner program.

Trace mode (already implemented) provides full step-by-step stack state
for deep debugging.

## Risks

- **Scope creep**: "just a small interpreter" → error handling, debugging,
  tracing, profiling. Discipline needed to keep it ~200 lines. The
  complexity budget goes into driver words, not the VM.
- **Two execution models in mu**: mu already has a plugin protocol (NDJSON
  process). Adding a second runtime means two ways to do things. Need clear
  guidance: stack programs for simple orchestration glue, plugin binaries
  for heavy lifting.
- **CUE evaluation cost**: Large programs-as-CUE-values could slow
  unification. Worth benchmarking with realistic method sizes (10-50 ops).
- **Stack debugging**: Stack traces are less intuitive than variable-based
  debugging. Mitigate with a `trace` mode that prints stack state after each
  word.
- **Agent reliability**: Even though concatenative is simpler than
  imperative, agents will still produce incorrect programs. The CUE
  vocabulary check catches unknown words but not wrong word order. Testing
  and `trace` mode are the safety nets.

## Prior Art

- **Swamp** (swamp.club) — TypeScript models with methods, executed via
  Deno. Agent-first design. Programs-as-code rather than programs-as-data.
- **Factor** — mature concatenative language with the vocabulary/combinator
  hierarchy this design draws from.
- **Joy** — pure concatenative language, quotations as the only control
  structure. Closest ancestor to this design.
- **Forsp** — call-by-push-value concatenative. Elegant but too complex
  for CUE embedding (closures, environments).
- **Retro** — sigil-based Forth. Sigils redundant when the program is
  already typed CUE data.

## Next Steps

1. ~~Create `~/dev/go/pith/`~~ — **DONE.** VM complete, 116 tests passing.
2. ~~Define CUE vocabulary schemas~~ — **DONE.** `cue.cue` ships
   `#Program`/`#Op` with all tier unions.
3. ~~Q1 Field ref resolution~~ — **DONE.** `VM.SetContext()` + dispatch fallback.
4. ~~Q2 Execution entry point~~ — **DONE.** Deferred; mu owns trigger.
5. ~~Q3 Driver word adapters~~ — **DONE.** `internal/pithdriver/` package.
6. ~~Q4 mu architecture~~ — **DONE.** No mismatch; Go coordinator confirmed.
7. ~~Q5 Missing vocabulary~~ — **DONE.** `group-by` + `flatten` implemented.
8. ~~Q6 Error reporting~~ — **DONE.** Op index in errors.
9. ~~Q7 String literals~~ — **DONE.** Single-quote sigil prefix.
10. ~~Example programs~~ — **DONE.** 5 integration tests in
    `internal/pithdriver/examples_test.go`: fleet summary (query +
    group-by), filter by status, count by origin, schema discovery,
    field ref query. Agent-authoring hypothesis holds for these cases.
11. ~~mu integration~~ — **DONE.** Full pith VM support in mu:
    plan phase (ActionBuffer + action/emit), transform phase (synthetic
    `_transform` action injection), execute phase (getOutput closure +
    http/exec/cas drivers). 427 tests pass.
12. Add `pudl exec` CLI for testing/debugging methods (informed by mu
    usage patterns).
13. Implement deferred words as needed: `format/*` (mu driver),
    `diff` (drift detection), `schema/match`, `schema/infer`.
14. Write an end-to-end mu target that uses inline pith Plan + Transform
    to validate the full flow.
