❯ and whats your take on how mu should use this?

⏺ mu's Current Execution Model

  mu has three layers:

  1. Targets — declared in mu.cue, specify toolchain + config
  2. Plugins — external binaries that receive targets, emit ActionSpec DAGs via NDJSON
  3. Actions — hermetic shell commands with CAS-cached inputs/outputs, executed in sandbox

  Plugin protocol is powerful but heavyweight. Every plugin is a separate binary with process lifecycle, NDJSON parsing, discover/plan/observe methods. For "call an API and
  transform the JSON" — that's a lot of machinery.

  Where the VM Fits: Three Levels

  Level 1: Actions as Stack Programs

  Most natural integration. mu actions currently have a command field (shell string). Add an alternative: body field (stack program list).

  // Current: shell command action
  action: {
      command: "curl -s https://api.example.com/vms | jq '.items'"
      inputs: [...]
      outputs: [...]
  }

  // New: stack program action
  action: {
      body: [
          "https://api.example.com/vms", "http/get",
          "items", "get",
      ]
      inputs: [...]
      outputs: [...]
  }

  Executor checks: body present → interpret with VM. command present → shell exec in sandbox. Caching works identically — hash(body + input_digests) instead of hash(command +
  input_digests).

  What this buys: hermetic, cacheable actions without shelling out. No curl | jq pipelines that break on edge cases. The VM handles JSON natively.

  Level 2: Plan Methods as Stack Programs

  This is the bigger win. Currently, plugins implement plan as Go/Python/whatever code compiled into a binary. A stack program could replace simple plugins entirely:

  // Instead of a plugin binary, the plan logic is inline
  target: {
      name: "//infra/dns-record"
      plan: [
          "config", "get",                    // get target config
          "dup", "record_type", "get",         // extract record type
          "A", "eq",
          [
              // A record: one API call
              ["host", "get"], ["ip", "get"], "dns/create-a",
          ],
          [
              // CNAME: different API call
              ["host", "get"], ["target", "get"], "dns/create-cname",
          ],
          "if",
          "action/emit",                       // emit ActionSpec to DAG
      ]
  }

  The coordinator sees plan field → interprets it → collects emitted actions → merges into DAG. No plugin process spawned.

  The key word here is action/emit. It's a driver word that doesn't execute an action — it declares one into the DAG. The plan program runs at planning time, producing action
  specs. The actions run later during execution. This preserves mu's plan/execute split.

  Level 3: Orchestration Glue Between Targets

  mu's DAG handles parallelism and dependencies. But sometimes you need glue logic between targets — transform output of target A before feeding to target B. Currently this
  requires a plugin or shell script.

  Stack programs as inter-target transforms:

  target: {
      name: "//deploy/config"
      depends: ["//infra/vpc", "//infra/db"]
      transform: [
          "//infra/vpc", "target/output", "vpc_id", "get",
          "//infra/db", "target/output", "endpoint", "get",
          // Build config from dependency outputs
          "connection_string", "format/db-url",
          "config", "swap", "db_url", "set",
      ]
  }

  Driver Words for mu Context

  // HTTP
  http/get    ( url -- response )
  http/post   ( url body -- response )
  http/put    ( url body -- response )
  http/delete ( url -- response )
  http/header ( key value -- )           // set header for next request

  // Process execution
  exec/run    ( [args] -- stdout )
  exec/shell  ( cmd -- stdout )

  // CAS (content-addressed store)
  cas/store   ( data -- digest )
  cas/fetch   ( digest -- data )

  // Secrets (integrates with mu's sealed inputs)
  secret/resolve  ( ref -- value )       // "pass:deploy/token" → actual token

  // DAG construction (plan-time only)
  action/emit     ( spec -- )            // emit ActionSpec into DAG
  action/depends  ( target -- )          // declare dependency

  // Target cross-references
  target/output   ( name -- data )       // read dependency's output
  target/config   ( -- config )          // current target's config

  // JSON/data (same as pudl context)
  get set has? keys values merge path pick omit

  What Stays as Plugins

  Stack programs replace simple plugins. Complex plugins stay as binaries:

  ┌──────────────────────────────────────────────────┬───────────────┬───────────────┐
  │                     Use Case                     │ Stack Program │ Plugin Binary │
  ├──────────────────────────────────────────────────┼───────────────┼───────────────┤
  │ API call + transform                             │       ✓       │               │
  ├──────────────────────────────────────────────────┼───────────────┼───────────────┤
  │ File write with permissions                      │       ✓       │               │
  ├──────────────────────────────────────────────────┼───────────────┼───────────────┤
  │ Config templating                                │       ✓       │               │
  ├──────────────────────────────────────────────────┼───────────────┼───────────────┤
  │ Go compilation                                   │               │       ✓       │
  ├──────────────────────────────────────────────────┼───────────────┼───────────────┤
  │ Docker build                                     │               │       ✓       │
  ├──────────────────────────────────────────────────┼───────────────┼───────────────┤
  │ Terraform apply                                  │               │       ✓       │
  ├──────────────────────────────────────────────────┼───────────────┼───────────────┤
  │ Anything needing subprocess streaming            │               │       ✓       │
  ├──────────────────────────────────────────────────┼───────────────┼───────────────┤
  │ Anything needing persistent state across actions │               │       ✓       │
  └──────────────────────────────────────────────────┴───────────────┴───────────────┘

  Rule of thumb: if the logic is "call API, transform data, emit result" — stack program. If it needs a toolchain, long-running process, or complex I/O — plugin binary.

  The Interesting Emergent Property

  When both pudl and mu share the VM:

  pudl: "what VMs exist?"
    → catalog/query → filter → map → result

  mu: "make this VM exist"
    → target/config → proxmox/create → action/emit

  pudl: "did it drift?"
    → catalog/query → definition/get → diff → drift/report

  mu: "fix the drift"
    → drift actions from pudl → proxmox/update → converge

  Same language, same data model (JSON on stack), different driver words. An agent that learns to write programs for pudl queries can immediately write programs for mu actions. The
   vocabulary is the only difference — and it's introspectable from CUE.

  This is the real payoff. Not just "a nicer way to write plugins" but a shared computational medium between the knowledge layer and the execution layer that an agent can operate
  fluently across both.

  Ready for the research doc?

✻ Sautéed for 1m 10s

❯ yes - write all of this to the research doc

⏺ Writing the research doc now.

  Listed 2 directories (ctrl+o to expand)

⏺ Write(docs/research/concatenative-vm.md)
  ⎿  Wrote 545 lines to docs/research/concatenative-vm.md
       1 # Concatenative VM for pudl and mu
       2
       3 Research document exploring an embedded stack-based (concatenative) language
       4 for defining methods on models in CUE, interpreted by a small Go runtime.
       5
       6 ## Motivation
       7
       8 Both pudl and mu need a way for agents to author small programs:
       9 - **pudl**: data transforms, computed views, enrichment pipelines over catalog data
      10 - **mu**: lightweight plugin logic, API call orchestration, inter-target glue
     … +535 lines (ctrl+o to expand)

⏺ Research doc written to docs/research/concatenative-vm.md. Covers all four topics we discussed: language choice rationale (plain concatenative over Retro/Forsp), combinator
  vocabulary design (4 tiers), pudl integration (read-only knowledge ops), and mu integration (3 levels from actions to plan methods to orchestration glue).

✻ Cooked for 2m 22s

❯ okay now one more question - should we isolate this as a library? i recently created 'ewe' (lives at ~/dev/go/ewe) to encapsulate the custom cue function that pudl has in its
  `op` package. it seems like this functionality could either be built on top of ewe as another library unto itself (it should at least be a separate internal package within
  pudl), whats your take?

⏺ Explore(Explore ewe project at ~/dev/go/ewe)
  ⎿  Done (32 tool uses · 57.8k tokens · 58s)
  (ctrl+o to expand)

  Listed 1 directory (ctrl+o to expand)

⏺ Good context. ewe is a CUE AST rewriter for custom functions — op.#Name & { args: [...] } → { args: [...], result: <value> }. The concatenative VM is a different beast entirely.

  My Take: Separate Library, Not Built on ewe

  ewe and the VM solve fundamentally different problems:

  - ewe: single function calls embedded in CUE, resolved at CUE eval time by AST rewriting. Stateless — each call is independent. The output is a CUE value.
  - VM: multi-step programs with a stack, control flow, and state threading. Runs after CUE evaluation. The CUE value describes the program; a Go interpreter executes it.

  Building the VM on top of ewe would be like building a scripting language on top of a template engine. They operate at different layers:

  CUE source
    → ewe rewrites function calls (AST level)
    → CUE evaluates (unification)
    → VM programs extracted as concrete CUE values
    → Go interpreter runs them (runtime level)

  They're sequential in the pipeline, not nested. ewe resolves op.#Hash & { args: ["data"] } into a concrete string during CUE eval. The VM takes a concrete body: [...] list and
  interprets it. ewe could produce inputs to VM programs (computed args), but the VM itself isn't a CUE function.

  Where It Should Live

  Three options, ranked:

  1. Standalone module at ~/dev/go/pith/ (recommended)

  Same pattern as ewe — extracted into its own module. Both pudl and mu import it. Name suggestion: pith (the core/essence, also short).

  pith/
  ├── vm.go          # VM struct, stack, dispatch loop
  ├── word.go        # Word type, driver registration
  ├── builtins.go    # Core words (dup, swap, each, map, filter, bi...)
  ├── data.go        # JSON data words (get, set, path, merge...)
  ├── cue.go         # Extract programs from CUE values
  └── pith_test.go

  Why standalone: mu needs to import it. If it lives inside pudl (even as pkg/), mu depends on pudl — that's a coupling direction that doesn't exist today and shouldn't. ewe proved
   the pattern: extract shared capability into its own module.

  2. pkg/forthvm/ inside pudl

  Works short-term. mu can't import it without depending on pudl's module. Fine if mu integration is distant future, but the research doc suggests it's a near-term goal.

  3. internal/ inside pudl

  Too restrictive. Can't be imported by mu or anything else. Only makes sense if this is purely a pudl experiment with no mu integration planned.

  The Relationship Between All Three

  ewe (CUE function execution)
    ↓ produces concrete CUE values
  pith (concatenative VM)
    ↓ interprets program lists from CUE
  pudl/mu driver words (domain logic)
    ↓ registered by each tool

  ewe and pith are peers — both are Go libraries that extend what you can do with CUE, but at different layers. pudl and mu both import whichever ones they need.

  pudl imports: ewe (custom CUE functions) + pith (VM for queries)
  mu imports:   pith (VM for actions/plans)

  No dependency between ewe and pith. No dependency between pudl and mu. Clean diamond.

  Want me to update the research doc with this architecture decision?
