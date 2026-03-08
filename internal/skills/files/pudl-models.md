# Defining PUDL Models

Models compose CUE schemas with operational behavior: methods, sockets, authentication, and metadata.

## Model Structure

Models are CUE files that reference schemas and add behavior:

```cue
package mymodels

import "pudl.schemas/pudl/model"

// Resource shape (the data schema)
#MyResource: {
    id:     string
    name:   string
    status: "active" | "inactive"
    config: {
        key: string
        value: _
    }
    ...
}

// Model definition
#MyModel: model.#Model & {
    schema: #MyResource

    metadata: model.#ModelMetadata & {
        name:        "my_resource"
        description: "Manages custom resources"
        category:    "custom"
        icon:        "box"
    }

    methods: {
        list: model.#Method & {
            kind:        "action"
            description: "List all resources"
            returns:     [...#MyResource]
            timeout:     "30s"
        }
        create: model.#Method & {
            kind:        "action"
            description: "Create a new resource"
            inputs: {
                name:   string
                config: _
            }
            returns: #MyResource
            timeout: "1m"
            retries: 1
        }
        validate_access: model.#Method & {
            kind:        "qualification"
            description: "Check API access credentials"
            returns:     model.#QualificationResult
            blocks:      ["list", "create"]
        }
    }

    sockets: {
        api_endpoint: model.#Socket & {
            direction:   "input"
            type:        string
            description: "API base URL"
            required:    true
        }
        resource_id: model.#Socket & {
            direction:   "output"
            type:        string
            description: "ID of created resource"
        }
    }

    auth: model.#AuthConfig & {
        method: "bearer"
    }
}
```

## Categories

Standard categories: `compute`, `storage`, `network`, `security`, `data`, `custom`.

## Authentication Methods

- `bearer` — Bearer token authentication
- `sigv4` — AWS Signature Version 4
- `basic` — HTTP Basic auth
- `custom` — Custom authentication logic

## Extension Models

Place models in `extensions/models/` under the schema path for automatic discovery. Extension models are found by `pudl model list` and `pudl model search` alongside built-in models.

```
~/.pudl/schema/
  extensions/
    models/
      mycompany/
        custom_service.cue
```

## Scaffold Command

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

## Method Lifecycle

Methods have a `kind` that determines when they run:

1. **Qualifications** run first. If any fail, the action is aborted.
2. **Action** runs the core operation.
3. **Attributes** compute derived values after the action.
4. **Codegen** transforms output to other formats.

The `blocks` field on qualifications declares which methods they gate.

## Socket Direction

- **Input sockets** receive values from other definitions or configuration
- **Output sockets** provide values that other definitions can consume
- Socket wiring creates the dependency graph between definitions
