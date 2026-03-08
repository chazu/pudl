# Model Authoring Guide

Models compose CUE schemas with operational behavior. A model references one or more schemas and adds methods, sockets, authentication, and metadata. Schemas remain pure data shapes — models layer behavior on top.

## File Structure

Model files are CUE files that import the `pudl/model` package and define one or more `#Model` values:

```cue
package mypackage

import "pudl.schemas/pudl/model"

#MyModel: model.#Model & {
    schema:   #MyResourceSchema
    metadata: model.#ModelMetadata & { ... }
    methods:  { ... }
    sockets:  { ... }  // optional
    auth:     model.#AuthConfig & { ... }  // optional
}
```

Models live in the schema repository (`~/.pudl/schema/`) alongside schemas. The `#Model` type enforces the required structure.

## Required Fields

### `schema`

Reference to the CUE schema that defines the resource shape. Can be an inline definition or an imported schema:

```cue
// Inline schema
schema: {
    id:   string
    name: string
}

// Or import an existing schema
import "pudl.schemas/aws/ec2"
schema: ec2.#Instance
```

### `metadata`

Descriptive information about the model:

```cue
metadata: model.#ModelMetadata & {
    name:        "ec2_instance"
    description: "AWS EC2 compute instance"
    category:    "compute"  // compute, storage, network, security, data, custom
    icon:        "server"   // optional
}
```

### `methods`

A map of method names to `#Method` definitions. Every model must have at least one method.

## Method Kinds

### `action` (default)

CRUD and custom operations:

```cue
create: model.#Method & {
    kind:        "action"
    description: "Launch a new instance"
    inputs:      { InstanceType: string, ImageId: string }
    returns:     #MyResource
    timeout:     "2m"
    retries:     1
}
```

### `qualification`

Precondition checks that gate other methods. Return `#QualificationResult`:

```cue
valid_credentials: model.#Method & {
    kind:        "qualification"
    description: "Verify credentials are valid"
    returns:     model.#QualificationResult
    blocks:      ["create", "delete"]  // gates these methods
}
```

When a method is invoked, all qualifications that list it in `blocks` run first. If any return `passed: false`, the method is aborted.

### `attribute`

Computed value derivation. Runs after actions:

```cue
compute_cost: model.#Method & {
    kind:        "attribute"
    description: "Calculate estimated monthly cost"
    returns:     { amount: number, currency: string }
}
```

### `codegen`

Output transformation to other formats:

```cue
to_terraform: model.#Method & {
    kind:        "codegen"
    description: "Generate Terraform HCL"
    returns:     string
}
```

## Method Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `kind` | string | `"action"` | Method type: action, qualification, attribute, codegen |
| `description` | string | - | Human-readable description |
| `inputs` | struct | - | Input parameters |
| `returns` | any | - | Return type |
| `timeout` | string | `"5m"` | Execution timeout (e.g., "30s", "2m", "1h") |
| `retries` | int | `0` | Number of retries (0-5) |
| `blocks` | list | - | For qualifications: methods this check gates |

## Sockets

Typed input/output ports for inter-component data flow:

```cue
sockets: {
    vpc_id: model.#Socket & {
        direction:   "input"
        type:        string
        description: "VPC ID for instance placement"
        required:    true  // default
    }
    instance_id: model.#Socket & {
        direction:   "output"
        type:        string
        description: "ID of the managed instance"
        required:    false
    }
}
```

- **Input sockets** are like function arguments — what this model needs from others.
- **Output sockets** are like function returns — what this model provides to others.
- Socket types are CUE values (primitives or schemas).
- Sockets enable wiring between definitions.

## Authentication

Configure how the model authenticates with external services:

```cue
auth: model.#AuthConfig & {
    method: "sigv4"  // bearer, sigv4, basic, custom
}
```

## Complete Example

```cue
package mypackage

import "pudl.schemas/pudl/model"

#StorageBucket: {
    name:     string
    region:   string
    acl:      "private" | "public-read" | *"private"
    ...
}

#StorageBucketModel: model.#Model & {
    schema: #StorageBucket

    metadata: model.#ModelMetadata & {
        name:        "storage_bucket"
        description: "Cloud storage bucket"
        category:    "storage"
    }

    methods: {
        create: model.#Method & {
            kind:        "action"
            description: "Create a new storage bucket"
            inputs:      { name: string, region: string }
            returns:     #StorageBucket
            timeout:     "1m"
        }
        check_name: model.#Method & {
            kind:        "qualification"
            description: "Verify bucket name is available"
            inputs:      { name: string }
            returns:     model.#QualificationResult
            blocks:      ["create"]
        }
    }

    sockets: {
        bucket_name: model.#Socket & {
            direction:   "output"
            type:        string
            description: "Name of the created bucket"
        }
    }

    auth: model.#AuthConfig & {
        method: "sigv4"
    }
}
```

## Extension Models

User-defined models can be placed in `extensions/models/` under the schema path for automatic discovery. Extension models appear in `pudl model list` and `pudl model search` alongside built-in models.

```
~/.pudl/schema/
  extensions/
    models/
      mycompany/
        custom_service.cue
```

## Effect Pattern

Instead of executing side effects directly, methods can return effect descriptions for the runtime to process:

```clojure
(defn run [args]
  {"result" "planned"
   "pudl/effects" [{"kind" "create"
                     "description" "Launch EC2 instance"
                     "params" {"instance_type" (get args "InstanceType")}}]})
```

Effect kinds: `create`, `delete`, `update`, `http`, `exec`. With `--dry-run`, effects are listed but not executed, providing an audit trail.

## CLI Commands

```bash
pudl model list                    # List all models
pudl model list --category compute # Filter by category
pudl model list --verbose          # Show details
pudl model show <model-name>       # Show model details
pudl model search <query>          # Search models by keyword
pudl model scaffold <name>         # Generate model boilerplate
```

### Scaffold Command

Generate model boilerplate with `pudl model scaffold`:

```bash
pudl model scaffold myservice \
  --category custom \
  --methods list,create,delete \
  --sockets api_url:input,resource_id:output \
  --auth bearer
```

This creates:
- `models/<name>/<name>.cue` — Model CUE file
- `methods/<name>/<method>.clj` — Method stub files
- `definitions/<name>_def.cue` — Definition template
