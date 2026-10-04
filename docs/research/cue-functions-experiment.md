# CUE `functions` experiment — evaluation for pudl

Date: 2026-10-04
CUE version evaluated: `cuelang.org/go v0.18.0-alpha.2` (language `v0.18.0`)
pudl baseline: `cuelang.org/go v0.16.0`, commit `c65eb1a`
Proposal: <https://cuelang.org/issue/4484>

## TL;DR

- CUE v0.18.0-alpha.2 ships first-class, user-defined **functions** behind a
  per-file `@experiment(functions)` attribute: `func(a: int, b: int) -> int: a + b`.
  They are typed, support named/positional/required/optional parameters,
  defaults and partial application, and capture their lexical scope.
  **Recursion is a structural-cycle error**, so CUE is still not
  Turing-complete.
- Every claim in this report was checked against the alpha's evaluator. The
  prototypes are reproduced in [Appendix A](#appendix-a--prototype-sources).
- **What functions can simplify in pudl:** authoring-side boilerplate, meaning
  datalog rule construction, `_pudl` metadata blocks, schema family
  specialisation, `#Check` construction and system-model factories.
  Functions also give call-site arity and type checking that pudl lacks today.
  Prototypes work through pudl's existing Go loaders **without Go changes**.
- **What functions cannot simplify in pudl (yet):** the Go-side engines.
  Identity hashing, inference scoring, the datalog compiler and evaluator,
  binding resolution and mu orchestration all stay in Go. The alpha has **no Go
  API to call a CUE function** and **no bridge to register Go functions**
  (a Go bridge is planned in a follow-up proposal). The no-recursion rule also
  rules out moving transitive logic out of datalog.
- **Upgrade cost is real but small, and it is separate from functions.** pudl
  builds unchanged against v0.18.0-alpha.2. The stabilised `explicitopen`
  semantics break `#SystemModel` (22 test failures) until three embeddings
  become `#SealedInputs...`/`#SealedExecution...` and a file attribute is
  added. Thirteen test and fixture files also pin language `v0.14.0`, where that attribute
  is illegal. With those fixes the full suite matches the v0.16 baseline.
- **Recommendation:** do the v0.18 compatibility preparation once v0.18 is
  final. Pilot functions only in *authoring helpers* (rules, checks, model
  factories) and keep them out of the shipped bootstrap schemas until the
  experiment stabilises. Revisit the Go-side opportunities when the Go bridge
  lands. See [Recommendations](#7-recommendations).

---

## 1. What the feature is

### 1.1 Enabling

Functions are a *file-level* experiment:

```cue
@experiment(functions)

package rules
```

Files without the attribute parse exactly as before. Every file that *defines
or calls* a user function, or uses labeled arguments, needs the attribute.
Calling with labels in a file without it fails with
`labeled arguments require @experiment(functions)`. This includes `cue eval -e`
expressions, which have no attribute.

### 1.2 Syntax and semantics (verified against `cue/testdata/functions/native.txtar`)

| Feature | Syntax | Notes |
|---|---|---|
| Literal | `func(a: int, b: int) -> int: a + b` | `-> T` is a result constraint, enforced on every call |
| Positional / labeled call | `f(1, 2)`, `f(a: 1, b: 2)`, `f(1, b: 2)` | arity is checked: "too many positional arguments", "missing argument b", "unknown argument c" |
| Positional-only | `func(_~x: int)` | uses the now-stable postfix alias syntax |
| Anonymous param | `func(int, a: int)` | |
| Required (label-only) | `func(a!: int)` | `f(4)` → "missing required argument a" |
| Optional | `func(b?: int)` | referencing an absent optional is *incomplete* |
| Default | `func(a: int = 5)` | applies only when the caller omits the argument. A default inherited via a reference (`#Port: int \| *80`) does **not** make the param omittable |
| Partial application | `add(1, ...)` | yields a function over the remaining parameters. Partials chain |
| Function types | `T: func(a: int, ...) -> number` | a bodyless signature acts as a constraint that can be unified with an implementation (`T & (func(a: int, b: int) -> int: a + b)`) |
| Closures / currying | `func(x: int) -> (func(y: int) -> int): func(y: int) -> int: x + y` | lexical scope; constraints resolve in the *declaring* scope |
| Recursion | `fib: func(n: int) -> int: fib(n-1) + fib(n-2)` | **error: structural cycle**, both direct and mutual |
| Builtins | `pkg/strings/pkg.cue` etc. now declare stdlib signatures as function types, for example `MaxRunes: (func(max: int) -> validator(string)) \| (func(s: string, max: int) -> bool)` | |

Restrictions found while prototyping that the proposal summary doesn't spell
out:

1. **A return type or parameter constraint cannot reference a parameter.**
   `func(n: string) -> {name: n}` fails with "cannot refer to parameter "n" in a
   parameter constraint or return type". Use `-> _: {name: n}` (constraint `_`,
   body `{name: n}`).
2. **A parameter default cannot reference a sibling parameter.**
   `func(name: string, query: string = name)` is rejected. This is the
   "query defaults to name" pattern, and it has to be spelled out at each
   call site.
3. **Function values cannot be exported to JSON.** A *visible* field holding a
   function makes `cue export` fail ("cannot convert value … of type
   *adt.FuncValue to JSON"). Helpers must live in hidden (`_f`) fields or
   definitions.
4. **Results are concrete, so defaults are consumed.** A helper that sets
   `resource_type: resource` with a concrete string argument produces a
   concrete value that a later specialisation cannot override. pudl's
   `#GitRepository` relies on `resource_type: string | *"git.repository"`
   being narrowed by `#GitHubRepository`. This keeps working only if the caller
   passes the disjunction as the argument:
   `_meta(string | *"git.repository", …)`.
5. **Errors are reported at the parameter/body site, not the call site.**
   Passing `severity: 3` reports `severity: conflicting values 3 and string`
   with positions inside the helper. The calling field (`badCheck`) is
   listed only as one of several positions. Diagnostics are adequate but
   less direct than a plain field conflict.

### 1.3 Go API status

- `cue.Value` has **no method to call a function value** from Go. pudl's Go code
  can only consume *results* of calls made inside CUE.
- There is **no mechanism to register Go functions** as CUE functions. The
  proposal defers this to a follow-up "Go bridge".
- `ctx.CompileString` honours the per-file `@experiment(functions)`
  attribute, so pudl's `datalog.ParseRules` (which compiles each rule file with
  `CompileString`) needs no changes.

---

## 2. How pudl uses CUE today (relevant surfaces)

| Surface | Where | Go consumer |
|---|---|---|
| Resource schemas with `_pudl` metadata (51 blocks in bootstrap) | `internal/importer/bootstrap/pudl/*/*.cue`, copied to `.pudl/schema/pudl/` | `internal/validator/cue_loader.go:176` iterates `#` definitions, keeps those with `_pudl` or list kind |
| Datalog rules as CUE data (`head`/`body` with `$Var` strings) | `bootstrap/pudl/rules/{rules,convergence}.cue` | `internal/datalog/loader.go:89` `ParseRules`: per-file `CompileString`, top-level non-definition fields with `head` and `body` |
| System models (`#SystemModel`, `#PluginObserve`, `#EweTarget`, `#Check`, sealed I/O) | `internal/systemmodel/schema.cue` (go:embed) plus the workspace copy | `internal/systemmodel/*`, `internal/wiring` resolve `inputs`/`bindings`, elaborate templates, decode |
| Identity derivation | `internal/identity/extract.go`, `resource_id.go` | dotted-path extraction from `identity_fields`, canonical JSON, SHA256 → proquint |

---

## 3. Opportunities, ranked

### 3.1 Datalog rule constructors (high value, low risk). Prototype verified

The shipped rules spell every atom out in full:

```cue
depends_transitive_rec: {
	head: {rel: "depends_transitive", args: {from: "$A", to: "$C"}}
	body: [
		{rel: "model_depends_on", args: {from: "$A", to: "$B"}},
		{rel: "depends_transitive", args: {from: "$B", to: "$C"}},
	]
}
```

`convergence.cue` documents the "arg-key contract" in prose because nothing
enforces it ("facts, rule body atoms, and `pudl query` constraints must all
agree"). With functions, each relation becomes a typed constructor:

```cue
@experiment(functions)
package rules

_depends: func(from: #Term, to: #Term) -> #Atom: {rel: "model_depends_on", args: {"from": from, "to": to}}
_trans:   func(from: #Term, to: #Term) -> #Atom: {rel: "depends_transitive", args: {"from": from, "to": to}}
_rule:    func(head: #Atom, body: [...#Atom]) -> #Rule: {"head": head, "body": body}

depends_transitive_base: _rule(_trans("$A", "$B"), [_depends("$A", "$B")])
depends_transitive_rec:  _rule(_trans("$A", "$C"), [_depends("$A", "$B"), _trans("$B", "$C")])
cyclic:                  _rule({rel: "cyclic", args: model: "$A"}, [_trans("$A", "$A")])
```

Verified results:

- `cue export` produces output identical in structure to today's rules.
- **pudl's unmodified `datalog.ParseRules`**, built against v0.18.0-alpha.2,
  parses all three rules with the expected heads and bodies.
- A misspelled key (`_depends(frm: "$A", to: "$B")`) or a missing argument
  (`_trans("$A")`) is now a **compile-time error** ("missing argument to").
  Today the same typo is a silently non-joining atom that returns no rows.
- A *visible* helper (`mk: func…`) does not break `ParseRules`: the loader's
  `extractRule` fails on it and skips it. Hidden helpers are still the right
  convention because visible ones break `cue export`.

What it simplifies: it removes the arg-key drift class of bug and shrinks rule
files. The `$`-string variable convention and the Go compiler are unchanged.

Limits:

- Helpers can't be shared across rule files. `LoadRulesFromPaths` compiles
  each file in isolation with `CompileString`, without package loading, so
  every file must define its own constructors. Sharing them needs the rule
  loader to switch to `load.Instances` (a Go change, and arguably desirable
  anyway).
- Constructors can't express anything the engine doesn't already support.
  Recursion in rules is still datalog's job, because CUE functions can't
  recurse.

### 3.2 `_pudl` metadata and family specialisation (medium value, medium risk). Prototype verified

Every one of the 51 `_pudl` blocks repeats the same four keys. Two hosted-git
specialisations duplicate the same `base_schema` and name-prefix pattern.
Prototype:

```cue
_meta: func(resource: string, identity: [...string], tracked: [...string] = [], type: string = "base") -> #PudlMeta: {
	schema_type: type, resource_type: resource, identity_fields: identity, tracked_fields: tracked
}

_hostedRepo: func(host: string, resource: string) -> _: {
	_pudl: {resource_type: resource, base_schema: "pudl/git.#GitRepository"}
	name: =~"^\(strings.Replace(host, ".", "\\.", -1))/"
}

#GitRepository: {
	_pudl: _meta(string | *"git.repository", ["name"], ["default_branch"]) & {base_schema?: string}
	...
}
#GitHubRepository: #GitRepository & _hostedRepo("github.com", "git.repository.github")
```

Verified: the specialisation narrows `resource_type`, sets `base_schema`, and
rejects `gitlab.com/x` ("out of bound =~"^github\.com/""). The first attempt
also confirmed **restriction 4** above: passing a plain `"git.repository"`
made the specialisation conflict, and the default had to be passed as a
disjunction.

Why the risk is medium: these schemas are the most widely read CUE in pudl.
Users copy them, the schema loader, `schemagen`, inference and `pudl schema`
commands read them, and a `_pudl: _meta(...)` call hides the metadata from
anyone reading the file. The savings per schema are about 3 lines. A
`#PudlMeta` *definition* (which pudl lacks today and which needs no
experiment) gives most of the validation benefit. **Recommendation: add
`#PudlMeta` now. Use `_meta` only in user-space schemas until functions are
stable.**

### 3.3 Check and system-model constructors (high value for authors). Prototype verified

`#Check` has five required fields. Most checks are "relation must be empty,
warn". A constructor with defaults:

```cue
_mustBeEmpty: func(name: string, query: string, severity: string = "warn", message: string) -> #Check: {
	"name": name, "query": query, expect: "empty", "severity": severity, "message": message
}
checks: [_mustBeEmpty("cyclic", "cyclic", message: "dependency cycle")]
```

(Restriction 2 forces `query` to be passed explicitly. It can't default to
`name`.)

System-model factories give model authors a typed way to stamp out similar
models. This one is verified against the real `#SystemModel` from
`.pudl/schema`:

```cue
_inventoryModel: func(name: string, script: string, inventory: string, desired: [...] = []) -> sm.#SystemModel: {
	"name": name
	plugins: [{"name": name, command: ["python3", script]}]
	populate: {plugin: name, differential: false, input: "inventory": inventory}
	"desired": desired
}
#GitInv:  _inventoryModel("git-inventory", "../obs.py", "current.json", [{"_schema": "git.repository", name: "local/demo"}])
#HostInv: _inventoryModel("host-inventory", "../host.py", "hosts.json")
```

The result still unifies with `#SystemModel`'s closedness rules: a typo
(`diferential`) inside the factory is rejected with "field not allowed". The
`#GitInv` result carries `_pudl` through `sm.#SystemModel`, so the schema
loader treats it as a model exactly as it treats `#GitInventory` today.

What it does **not** replace is pudl's `inputs` + `bindings` mechanism
(`internal/wiring`). Bindings are resolved by Go from *catalog snapshots*
under provenance, freshness and approval rules. A CUE function can't read the
catalog, and Go can't call a CUE function. Factories are compile-time
parameterisation, and bindings are run-time parameterisation, so the two are
complementary.

### 3.4 Identity key derivation in CUE (low value, not recommended)

It is possible:

```cue
_identityKey: func(schema: string, rec: {...}, fields: [...string]) -> string:
	schema + "|" + strings.Join([for f in fields {"\(rec[f])"}], "/")
```

Verified output: `"pudl/git.#GitRepository|github.com/chazu/pudl"`. But
`internal/identity` also handles dotted nested paths (needs a variable-depth
walk, and CUE functions can't recurse), arrays (first element), canonical JSON
ordering, SHA256 and proquint encoding. It must also agree byte-for-byte with
existing catalog IDs. Duplicating it in CUE would create two sources of
truth. **Keep identity in Go.**

### 3.5 Validator-style parameterised constraints (small, already mostly possible)

Functions allow parameterised constraints (`_hostedRepo("github.com", …)`), and
the stdlib's new `validator(T)` result type documents the pattern. Most of
pudl's needs (prefix regexes, enums, bounds) are already expressible with
plain constraints and interpolation. This is a convenience, not a
simplification.

### 3.6 What functions do *not* change

| pudl subsystem | Why functions don't help (now) |
|---|---|
| Datalog compiler and evaluator (`internal/datalog`) | Recursion is forbidden in CUE functions, and semi-naive fixpoint over SQLite is the right tool. Rules remain *data* |
| Inference heuristics (`internal/inference`) | Scoring runs over many candidate schemas in Go. Calling CUE functions from Go isn't possible |
| Identity and resource IDs (`internal/identity`) | See 3.4 |
| Binding resolution (`internal/wiring`) | Needs catalog access, provenance and approval semantics. No Go bridge exists |
| mu orchestration (`internal/mubridge`) | Generates and ingests mu artifacts in Go. No CUE-side computation to absorb |

---

## 4. The future Go bridge: what it could unlock

The proposal states that a follow-up will let Go functions be exposed to CUE
without WebAssembly. If that lands with a reasonable purity story, pudl could
*consider*:

- `pudl.identity(schema, record)` as a CUE-visible builtin backed by
  `internal/identity`. Schemas and models could then compute catalog keys
  (for example in `#ResourceRef`) with a single implementation.
- Catalog lookups for bindings (`pudl.lookup(model, schema, identity).path`).
  These would need to be impure or snapshot-pinned, and they would collide
  with the approval and provenance guarantees in `internal/wiring`. This is
  likely a non-goal.

Neither is actionable today.

---

## 5. Upgrade cost: moving pudl to CUE v0.18

The upgrade was tested end-to-end in a scratch worktree. This part applies
whether or not functions are adopted, because v0.18 stabilises two
experiments that change semantics:

| Step | Result |
|---|---|
| `go get cuelang.org/go@v0.18.0-alpha.2 && go mod tidy` | Builds cleanly. **No Go API breakage** in pudl. pudl already declares `go 1.26.2`, which meets v0.18's Go 1.26 requirement |
| `go test ./...` (v0.16 baseline) | 2 pre-existing failures: `internal/database TestNewCatalogDB` (nonexistent path; environment-dependent, not CUE) and `internal/mubridge TestIngestObserve_FailedIngestLeavesNoSnapshot` |
| `go test ./...` on alpha, no CUE source changes | +22 failures in `internal/systemmodel` and `internal/wiring`: `#SystemModel.populate: 5 errors in empty disjunction … field not allowed` |
| Cause | `explicitopen` is stable at language v0.18: embedding a closed definition closes the struct. `#PluginObserve`, `#EweTarget` and `#SealedExecution` embed `#SealedInputs`. The embedded `internal/systemmodel/schema.cue` is compiled with `CompileString`, which uses the **latest** language version, so it picks up v0.18 semantics even though workspaces pin v0.16 |
| Fix | `#SealedInputs` → `#SealedInputs...` (3 sites, exactly what `cue fix --exp=explicitopen,aliasv2` produces). The same file is also loaded inside workspaces pinned to v0.16, so it needs `@experiment(explicitopen)` at the top to be valid in both contexts |
| Second-order breakage | 13 test/fixture files (`cmd/*_test.go`, `internal/{inference,validator,doctor,mubridge}`, `test/testutil/temp_dirs.go`, `test/integration/infrastructure/suite.go`, `test/system/config_test.go`) create workspaces with `language: version: "v0.14.0"`. `explicitopen` can't be set before v0.15, so those workspaces failed to load schemas. One visible symptom: `TestImportNDJSON_LinuxSchemaRouting` silently fell back to the catch-all `#Item` schema. Bumping the fixtures to `v0.16.0` fixes this |
| After both fixes | Full suite matches the v0.16 baseline (only the 2 pre-existing failures) |
| `cue vet` of `.pudl/schema` at language v0.18 after `cue fix` | Clean. `cue fix` also reformats about 19 files and adds `@experiment(explicitopen)` attributes |

Observations worth acting on regardless of CUE version:

- **One embedded CUE file is compiled under two language versions.**
  `internal/systemmodel/schema.cue` is compiled by `CompileString` (latest)
  and also lives in workspaces pinned to `v0.16.0` (`internal/init/init.go:268`,
  `internal/repo/init.go:176`). Any future semantic change in CUE will
  diverge between `pudl run` and `pudl doctor`/schema validation. Consider
  compiling the embedded copy with an explicit language version
  (`cuecontext`/`cue/build` options) so both paths agree.
- **A schema-load failure silently degrades inference to `#Item`.** The
  Linux routing test caught it only because it asserts on the routed schema.
  `pudl doctor` should probably surface a module that fails to load.

---

## 6. Risks of adopting functions now

1. **Alpha plus experiment.** The syntax, error messages and semantics
   (defaults, partial application, attachment of labels across unified
   signatures) are still changing between alphas. The test data includes
   many "this used to …" regressions.
2. **Per-file opt-in spreads.** Every file that calls a helper needs the
   attribute, including files users write against shipped helpers.
3. **Toolchain coupling.** Users editing `.pudl/schema` with their own `cue`
   binary need ≥ v0.18 once any shipped file uses functions. Older CUE fails
   to parse `func(...)`.
4. **Readability of the shipped schemas.** Schemas are pudl's documentation
   and the template users copy. Indirection through helpers trades
   locality for brevity.
5. **Diagnostics.** Errors point into helper bodies (restriction 5).

---

## 7. Recommendations

| # | Action | When | Effort |
|---|---|---|---|
| R1 | Prepare for v0.18 semantics: apply the three `#Sealed*...` rewrites plus `@experiment(explicitopen)` in `internal/systemmodel/schema.cue` and the workspace copy; bump the 13 `v0.14.0` test fixtures to `v0.16.0`; run `cue fix --exp=explicitopen,aliasv2` on bootstrap schemas | When v0.18.0 final ships (or now, since the attribute is valid from v0.15) | Small |
| R2 | Pin the language version used by `CompileString` for embedded schemas, so CLI and workspace evaluation can't diverge | Independent of functions | Small |
| R3 | Add a plain `#PudlMeta` definition and use it to constrain every `_pudl` block | Now (no experiment needed) | Small |
| R4 | Pilot functions in **rule files**: typed relation constructors (`_depends`, `_trans`, `_rule`) in a user-space rules file, and document the pattern in `docs/datalog.md` | After v0.18 final; keep shipped bootstrap rules plain until the experiment stabilises | Small |
| R5 | Switch `LoadRulesFromPaths` from per-file `CompileString` to package loading, so constructors can be shared across rule files (and rule files can import schemas) | With R4 | Medium |
| R6 | Offer `_mustBeEmpty`-style `#Check` constructors and system-model factory examples in `docs/schema-authoring.md` and `examples/` | After v0.18 final | Small |
| R7 | Don't move identity, inference, datalog evaluation or binding resolution into CUE. Re-evaluate `pudl.identity` as a CUE builtin when the Go bridge proposal lands | Revisit later | n/a |

Net assessment: functions are a real ergonomic improvement for **authoring**
pudl's CUE (rules, checks, models), and they close the arg-key drift class
of rule bug. They don't change pudl's architecture: the Go engines stay as
they are. The v0.18 upgrade that would carry them is cheap, and it surfaced
two latent issues (dual language-version compilation and silent schema-load
degradation) worth fixing on their own.

---

## Appendix A — prototype sources

All prototypes were evaluated with `cue v0.18.0-alpha.2` (built via
`go install cuelang.org/go/cmd/cue@v0.18.0-alpha.2`).

### A.1 Rules (`rules.cue`), checked with `cue export` and pudl's `ParseRules`

```cue
@experiment(functions)

package rules

#Term: string | number | bool
#Atom: {rel: string, args: {[string]: #Term}}
#Rule: {name?: string, head: #Atom, body: [...#Atom] & [_, ...]}

_depends: func(from: #Term, to: #Term) -> #Atom: {rel: "model_depends_on", args: {"from": from, "to": to}}
_trans:   func(from: #Term, to: #Term) -> #Atom: {rel: "depends_transitive", args: {"from": from, "to": to}}
_rule:    func(head: #Atom, body: [...#Atom]) -> #Rule: {"head": head, "body": body}

depends_transitive_base: _rule(_trans("$A", "$B"), [_depends("$A", "$B")])
depends_transitive_rec:  _rule(_trans("$A", "$C"), [_depends("$A", "$B"), _trans("$B", "$C")])
cyclic: _rule({rel: "cyclic", args: model: "$A"}, [_trans("$A", "$A")])
```

Negative case: `bad: _rule(_trans("$A"), [_depends(frm: "$A", to: "$B")])`
gives `head: missing argument to`.

### A.2 Metadata, specialisation, identity, checks

```cue
@experiment(functions)
package p

import "strings"

#PudlMeta: {
	schema_type: "base" | "catchall" | "collection" | "list" | "policy"
	resource_type: string
	identity_fields: [...string]
	tracked_fields: [...string]
	base_schema?: string
}
_meta: func(resource: string, identity: [...string], tracked: [...string] = [], type: string = "base") -> #PudlMeta: {
	schema_type: type, resource_type: resource, identity_fields: identity, tracked_fields: tracked
}
_identityKey: func(schema: string, rec: {...}, fields: [...string]) -> string:
	schema + "|" + strings.Join([for f in fields {"\(rec[f])"}], "/")
_hostedRepo: func(host: string, resource: string) -> _: {
	_pudl: {resource_type: resource, base_schema: "pudl/git.#GitRepository"}
	name: =~"^\(strings.Replace(host, ".", "\\.", -1))/"
}
#GitRepository: {
	_pudl: _meta(string | *"git.repository", ["name"], ["default_branch"]) & {base_schema?: string}
	name: string
	default_branch: string
	...
}
#GitHubRepository: #GitRepository & _hostedRepo("github.com", "git.repository.github")

#Check: {name: string, query: string, expect: "empty" | "nonempty", severity: "info" | "warn" | "fail", message: string}
_mustBeEmpty: func(name: string, query: string, severity: string = "warn", message: string) -> #Check: {
	"name": name, "query": query, expect: "empty", "severity": severity, "message": message
}

checks: [_mustBeEmpty("cyclic", "cyclic", message: "dependency cycle"), _mustBeEmpty("orphan_subnet", "orphan_subnet", severity: "fail", message: "x")]
repo: #GitHubRepository & {name: "github.com/chazu/pudl", default_branch: "main"}
key:  _identityKey("pudl/git.#GitRepository", repo, repo._pudl.identity_fields)
meta: repo._pudl
```

Exported `key` is `"pudl/git.#GitRepository|github.com/chazu/pudl"`. `meta`
has `resource_type: "git.repository.github"` and
`base_schema: "pudl/git.#GitRepository"`.

### A.3 System-model factory

See §3.3. It was evaluated inside a copy of `.pudl/schema` (models package
importing `pudl.schemas/pudl/systemmodel@v0`).
