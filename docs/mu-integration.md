# pudl + mu Integration

pudl detects drift. mu fixes it.

- **pudl** models desired state in CUE, imports actual state, and computes diffs.
- **mu** takes desired-state targets and converges them using resource-type plugins.

The two tools communicate through mu.json — pudl generates it, mu consumes it.
There is no code-level dependency between them.

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
