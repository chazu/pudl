# pudl + mu Integration

The two tools collaborate in two directions:

- **pudl → mu** (drift convergence): pudl models desired state in CUE,
  imports actual state, computes diffs, and emits a `mu.json` for mu to
  consume and converge.
- **mu → pudl** (data import): mu plugins produce structured data; that
  data flows into pudl's catalog, optionally tagged with a CUE schema
  reference declared by the plugin.

There is no code-level dependency between the two binaries — they
communicate through files (mu.json, plugin output, schema sidecars)
that either side can author independently.

## Direction 1: drift convergence (pudl → mu)

## The Flow

```
CUE definitions (desired state)
        │
        ▼
pudl import (observe actual state)
        │
        ▼
pudl drift check (compute diff)
        │
        ▼
pudl export-actions (emit mu.json)
        │
        ▼
mu build --config (converge via plugins)
```

## Example: Converging a Config File

### 1. Define desired state

Create a CUE definition in `~/.pudl/schemas/definitions/`:

```cue
package definitions

import "file"

app_config: file.#Config & {
    path:    "/tmp/myapp/config.toml"
    content: """
        [server]
        port = 8080
        host = "0.0.0.0"

        [database]
        url = "postgres://localhost:5432/myapp"
        """
    mode: "0644"
}
```

### 2. Import actual state and check drift

```bash
# Import the current file (or note it doesn't exist yet)
pudl import --path /tmp/myapp/config.toml

# Check for drift
pudl drift check app_config
```

Output:
```
Definition: app_config
Status: drifted
Differences:
  content: changed
  mode: changed
```

### 3. Export mu config

```bash
pudl export-actions --definition app_config > /tmp/converge.json
```

This produces:

```json
{
  "targets": [
    {
      "target": "//app_config",
      "toolchain": "file",
      "sources": ["/Users/you/.pudl/schemas/definitions/app.cue"],
      "config": {
        "path": "/tmp/myapp/config.toml",
        "content": "[server]\nport = 8080\nhost = \"0.0.0.0\"\n\n[database]\nurl = \"postgres://localhost:5432/myapp\"",
        "mode": "0644"
      }
    }
  ]
}
```

Note: the config contains the **desired state**, not the drift diff. The mu
plugin doesn't need to know what changed — it just makes the file match.

### 4. Converge with mu

```bash
mu build --config /tmp/converge.json //app_config
```

Output:
```
mu build //app_config
  building...
  ✓ 2 completed (0 cached), 0 failed in 0.2s
```

The file is now written with the correct content and permissions.

### 5. Verify convergence

```bash
# Re-import and check drift
pudl import --path /tmp/myapp/config.toml
pudl drift check app_config
```

Output:
```
Definition: app_config
Status: clean
```

## Export All Drifted Definitions

```bash
pudl export-actions --all > /tmp/converge.json
mu build --config /tmp/converge.json //...
```

This exports targets for every definition that has drifted, then mu converges
them all in a single build (with parallel execution where possible).

## How Schema Refs Map to Toolchains

pudl infers the mu toolchain from the CUE schema reference:

| Schema prefix | mu toolchain | What it does |
|--------------|-------------|-------------|
| `file.*`, `config.*` | `file` | Write files, set permissions |
| `ec2.*`, `s3.*`, `aws.*` | `aws` | AWS resource convergence |
| `k8s.*`, `kubernetes.*` | `k8s` | Kubernetes resource convergence |
| (unknown) | `generic` | Fallback |

Custom mappings can be provided to `ExportMuConfig()` in the Go API.

## BRICK Interface Enforcement

pudl validates that BRICK components satisfy the contracts defined by their
interfaces. This runs automatically as part of `pudl definition validate`.

### Defining an Interface

An interface declares a contract — the fields any implementing component must have:

```cue
package definitions

import "pudl.schemas/pudl/brick"

lint_interface: brick.#Interface & {
    name: "//interface/lint"
    kind: "interface"
    desc: "Contract for code linting targets"
    contract: {
        toolchain: "lint"
        config: {
            command: [...string]
        }
    }
}
```

### Implementing an Interface

Components declare which interface they implement via the `implements` field:

```cue
lint_go_vet: brick.#Target & {
    name:       "//lint/go-vet"
    kind:       "component"
    toolchain:  "lint"
    implements: "//interface/lint"
    config: {
        command: ["go", "vet", "./..."]
    }
}
```

### Validation

`pudl definition validate` checks two things:

1. **Schema validation** — each definition conforms to its CUE schema
   (e.g., `brick.#Target` fields are correct)
2. **Interface enforcement** — each component's fields unify with its
   interface's contract via CUE unification

```bash
$ pudl definition validate
  PASS  lint_go_vet
  PASS  lint_gofmt
  PASS  lint_interface

Results: 3 passed, 0 failed, 3 total

Interface enforcement: 1 interfaces, 2 components
  All components satisfy their interfaces.
```

When a component violates its interface:

```
  FAIL  //lint/bad (implements //interface/lint)
        field "toolchain": conflicting values "lint" and "wrong"
```

### How It Works

The interface checker:

1. Loads all definitions as CUE values
2. Identifies interfaces (`kind: "interface"` with a `contract` field)
3. Identifies components (targets with an `implements` field)
4. For each component, unifies it with its interface's contract
5. CUE unification naturally catches mismatches — conflicting values,
   missing required fields, and type errors

Components referencing non-existent interfaces produce warnings (orphans).

### The Split

- **pudl enforces** that components satisfy interface contracts (pre-deploy)
- **mu carries** `kind` and `implements` through build manifests (metadata)
- **mu does not enforce** contracts — it executes targets regardless of BRICK classification

This keeps mu simple (execute everything, ask no questions) while pudl
provides the safety net (validate before exporting to mu).

## Design Principles

**pudl doesn't execute.** It observes, models, and reports. Execution is mu's job.

**mu doesn't observe.** It takes a config and converges. Observation is pudl's job.

**Plugins are ignorant of pudl.** A mu file plugin works the same whether
the target was generated by pudl or written by hand. The plugin only sees
desired state as a JSON config object.

**No coupling.** pudl needs to know what config shape each plugin expects
(so it can translate CUE definitions correctly), but this is just schema
knowledge — the same thing pudl already manages.

---

## Direction 2: data import (mu → pudl)

mu plugins produce structured data. When that data flows into pudl, the
plugin can declare a CUE schema reference so pudl classifies the data
under a meaningful type instead of falling back to the catchall
`pudl/core.#Item`.

### The flow

```
mu plugin produces output
        │
        ├── data file (e.g. out.json)
        └── sidecar (out.json.schema.json) — declares the schema ref
        │
        ▼
pudl import --path out.json
        │
        ├── reads sidecar (or --schema flag)
        ├── attempts to resolve the ref
        │      ├── known   → classify (declared)
        │      ├── auto-register from vendored CUE → classify (auto_registered)
        │      └── unknown → infer + tag for `pudl reclassify` (unresolved)
        ▼
catalog row + item_schemas rows recording all classifications
```

### Sidecar shape

For data file `<path>`, the sidecar lives at `<path>.schema.json`:

```json
{
  "module":     "mu/aws",
  "version":    "v1",
  "definition": "#EC2Instance"
}
```

### Explicit override

For agentic or one-off use, the sidecar can be skipped — `pudl import
--schema mu/aws@v1#EC2Instance --path out.json` does the same thing.

### Multiple schemas per item

A single imported item can match multiple schemas (e.g.
`mu/aws#EC2Instance` *and* `cloud/compute#Instance`). The `item_schemas`
junction table represents this naturally — one row per (item,
schema_ref) pair, each with its own status and timestamp.

### Reclassification

When a schema reference is unknown at import time, pudl tags the item
with the unresolved ref. `pudl reclassify` rescans these rows and
upgrades them once the schema becomes resolvable. See the W4/W5
sections of `docs/plans/2026-05-04-feat-plugin-output-schemas-plan.md`
in the mu repo for the full design.

### Plugin author docs

If you're writing a mu plugin and want pudl to type-classify your
output, see `mu/docs/plugin-output-schemas.md`.
