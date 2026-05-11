What kind of models does swamp use? How does that compare with the prior system initiative product
6:07 PM
Claude responded: Good question — there's a clear lineage but a pretty radical shift in form factor.
Synthesized comparative analysis of model architectures

Good question — there's a clear lineage but a pretty radical shift in form factor.

Swamp's models are typed representations of external systems (cloud resources, CLI tools, APIs), where each model type defines metadata, arguments, methods, and inputs. They're TypeScript files that live in a .swamp/ directory inside a git repo. An AI agent (like Claude Code) writes the TypeScript, then runs swamp model method run to test it — Swamp compiles, executes, and stores the results as immutable, queryable data. So a "model" in Swamp is basically a typed wrapper around an external system that an agent can invoke and reason about.
GitHub
Keeb

The original SI product was a very different beast. It used a visual, interactive graph model to represent infrastructure components — the diagram wasn't just a visual reference but was the actual infrastructure. At its core was a programmable model of an IT environment providing an abstraction layer, essentially a high-fidelity digital simulation mapping relationships between components. Assets were defined by Schemas (with versioned Schema Variants), and components connected via sockets and subscriptions. It was a SaaS platform with a Figma-like multiuser canvas.
Adyog Blog + 2

The pivot story is pretty stark. Paul Stack explained that System Initiative spent over five years iterating on the visual infrastructure tool, then went from 17 employees to a five-person team building Swamp as a fully open-source CLI for AI agents. The key lesson was that a SaaS-first architecture meant constantly transmitting context between a central API and the user's agent, which was expensive regardless of optimization. That pushed them toward a local-first design where Swamp keeps data on the user's machine, close to where the agent operates.
Podbean
Podbean

So the conceptual thread — typed models of external infrastructure with methods you can invoke — carried over, but the delivery mechanism went from "visual graph in a SaaS" to "git-native files an AI agent manipulates via CLI." Pretty aligned with your interest in agentic SDLC tooling and the "paved road" approach, actually.
You said: What are the formal properties of these models?

What are the formal properties of these models?
6:08 PM
Claude responded: That blog post gives us a concrete look at the structure.
Synthesized formal model structure comprehensively

That blog post gives us a concrete look at the structure. A Swamp model is a TypeScript module that exports an object with these formal properties:
typescript

export const model = {
  type: "proxmox/api",      // namespaced type identifier
  version: 1,               // schema version
  inputAttributesSchema: InputSchema,  // typed inputs
  dataAttributesSchema: DataSchema,    // typed outputs
  methods: { authenticate, listVms, getVm }  // callable operations
};

Keeb

So the formal shape is: type (namespaced string), version (integer), inputAttributesSchema (describes what goes in), dataAttributesSchema (describes what comes out), and methods (named async functions that take input+data and return results).

Models are then instantiated via definitions — YAML files with specific configuration that support CEL expressions for dynamic values and cross-model references. So there's a type/instance split: the TypeScript model is the type, the YAML definition is the instance.
GitHub

The runtime properties are also interesting: methods are individually testable via swamp model method run, results are stored immutably in swamp's model system as queryable data, and the whole thing is compiled and executed via Deno.
Keeb

This is a pretty lightweight formal system compared to what you might expect. There's no explicit dependency graph between models, no socket/connection typing like old SI had, and no constraint propagation. The coordination happens at the workflow layer (jobs, steps, triggers) rather than being intrinsic to the model schema itself. CEL expressions in definitions are the main mechanism for cross-model data flow.

It's closer in spirit to something like a typed resource provider (think Pulumi providers or Terraform provider schemas) than to old SI's reactive graph. Which makes sense given the agent-first design — an LLM can reason about "call method A, use its output as input to method B" much more easily than it can reason about reactive constraint propagation across a visual hypergraph.
You said: Can something similar be done with cuelang in go?

Can something similar be done with cuelang in go?
6:11 PM
Claude responded: Yeah, and in some ways CUE is a more natural fit for the schema/constraint side than TypeScript + CEL.
Evaluated CUE's capability to replicate Swamp's model architecture

Yeah, and in some ways CUE is a more natural fit for the schema/constraint side than TypeScript + CEL. The mapping is pretty direct:

Schema definition — CUE's type lattice replaces both the TypeScript schema types and CEL validation. You'd define a model type as a CUE definition:
cue

#Model: {
    type:    string & =~"^[a-z]+/[a-z]+$"
    version: int & >=1
    input:   #InputSchema
    data:    #DataSchema
    methods: [string]: #Method
}

#Method: {
    description: string
    args?:       _
    returns:     _
}

CUE's closed structs and value lattice give you something Swamp's TypeScript schemas don't — the constraint propagation that old SI had. If you say memory: int & >=512 & <=65536, that's not just validation, it's a constraint that composes with other constraints via unification.

Definitions (instances) — this is where CUE really shines vs YAML+CEL. Instead of YAML files with CEL expressions spliced in, your definitions are just more CUE that unifies with the schema:
cue

import "models/proxmox"

proxmoxAuth: proxmox.#Model & {
    type: "proxmox/api"
    input: {
        host: "192.168.1.100"
        port: 8006
        user: vault.get("proxmox", "user")
    }
}

Cross-model references become regular CUE references instead of CEL string interpolation — they're checked at eval time, not stringly-typed. You get the DAG of dependencies for free from CUE's reference graph.

Methods / execution — this is where you'd need Go. CUE defines the shapes but doesn't execute side effects. The pattern would be something like:
go

type ModelRunner struct {
    schema cue.Value  // compiled CUE definition
}

func (m *ModelRunner) RunMethod(name string, ctx context.Context) (*Data, error) {
    // extract method spec from CUE value
    // execute Go function registered for that method
    // validate output against data schema via CUE
    // store immutably
}

You'd register Go functions as method implementations, and CUE would validate both the inputs and outputs. This is essentially what pudl is reaching toward — CUE as the schema/constraint layer, Go as the execution layer.

What you gain over Swamp's approach:

CUE's lattice-based unification means you can compose partial definitions. An org-level base config, a team-level overlay, and an instance-level override all merge via & with conflict detection — no inheritance hierarchy, just lattice meets. You also get CUE's vet for drift detection (does the live state still satisfy the constraints?), which is more principled than ad-hoc validation.

What's harder:

CUE's Go API has historically been somewhat painful for dynamic use cases — the cue/load and cuecontext APIs are powerful but verbose. And CUE intentionally has no general recursion or Turing-completeness, so any method logic has to live in Go, not in the schema language itself. Swamp sidesteps this by just letting you write TypeScript, which is more approachable for an AI agent that needs to author model code on the fly.

That last point is the real tradeoff: Swamp's design optimizes for "an LLM writes the model," so TypeScript is a pragmatic choice. A CUE+Go version would be more formally rigorous but harder for an agent to author — unless the agent is generating CUE definitions against fixed Go method implementations, which is actually a pretty clean split. The agent handles config/wiring (CUE), humans handle the effectful code (Go).
You said: What about using a small stack based language to hold method definitions in cue

What about using a small stack based language to hold method definitions in cue
6:12 PM
Claude responded: This is a really compelling idea.
Examined embedding stack-based DSL within CUE for method definitions

This is a really compelling idea. You'd be storing method bodies as data inside CUE — programs-as-values — which CUE is totally fine with since it doesn't care whether a string happens to be Forth. Then a small Go-side interpreter executes them.
cue

#Method: {
    description: string
    input:       #Schema
    output:      #Schema
    body:        [...#Op]  // stack program as structured data
}

#Op: string  // or a union if you want typed words

methods: listVms: #Method & {
    description: "List all VMs on a Proxmox node"
    input: { host: string, ticket: string }
    output: { vms: [...#VM] }
    body: [
        "input.host", "input.ticket",   // push from input
        "proxmox/auth",                  // authenticate
        "nodes", "proxmox/get",          // GET /nodes
        "flatten",
        "each", [
            "dup", "node", "get",
            "qemu", "proxmox/get",       // GET /nodes/{node}/qemu
        ],
        "concat",
        "output.vms", "set",
    ]
}

The nice thing about representing ops as a CUE list rather than a raw string is that CUE can validate the program structure — you can constrain the vocabulary:
cue

#BuiltinOp: "dup" | "drop" | "swap" | "over" |
            "each" | "concat" | "flatten" |
            "get" | "set" | "dup2"

#DriverOp: =~"^[a-z]+/[a-z]+$"  // namespaced driver calls

#Op: #BuiltinOp | #DriverOp | [...#Op] |  // quoted programs for combinators
     =~"^input\\." | =~"^output\\."       // field references

So CUE is doing static vocabulary checking on your program before it ever hits the interpreter. Not type checking the stack — that's hard without dependent types — but at least ensuring every word is known.

The Go interpreter stays tiny because the effectful words (proxmox/get, aws/describe, etc.) are registered as "drivers" — basically the same concept as Forth's vocabularies or wordlists:
go

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

Each driver is a Go package that registers its words. The interpreter is maybe 200 lines. The CUE definitions carry the programs. The programs compose by quotation and combinators (each, if, bi) rather than by naming — which means an agent can generate them by concatenation without worrying about scope or binding.

This actually addresses the "agent authoring" problem better than raw Go in some ways. A concatenative program is just a list of words — no variables, no control flow nesting, no syntax to get wrong. An LLM can generate ["input.host", "dns/lookup", "each", ["ping"], "filter"] more reliably than it can generate correct Go with error handling. And CUE validates the vocabulary at definition time.

The interesting deeper question is whether you make the quotation brackets ([...]) first-class in CUE's type system. You could define combinator signatures:
cue

#Combinator: {
    word: "each"
    expects: "quotation on top, sequence below"
    produces: "sequence of results"
}

That's documentation-as-data, which is useful for agent introspection — the agent can read the combinator specs from CUE to understand what's available. Not formal stack-effect declarations like Factor has, but enough for an LLM to reason about.

The Forth-in-CUE-in-Go stack also maps cleanly onto your tuplespace thinking. Each driver word could read/write tuples rather than calling APIs directly — "proxmox/vms", "ts/rd" reads a VM tuple from the space, and the interpreter doesn't care whether that tuple was populated by a live API call or a cached scan. The program stays the same; the tuplespace handles the indirection.


Claude is AI and can make mistakes. Please double-check responses.
