# Definition Authoring Guide

Definitions are named instances of models with concrete configuration and socket wiring. They live in `~/.pudl/schema/definitions/` as CUE files.

## File Structure

```
~/.pudl/schema/definitions/
├── simple_def.cue      # Simple definitions
├── http_def.cue        # HTTP endpoint definition
└── wired_defs.cue      # Definitions with socket wiring
```

All definition files use `package definitions`.

## Basic Definition

A definition unifies concrete values against a model's schema:

```cue
package definitions

import "pudl.schemas/pudl/model/examples"

my_simple: examples.#SimpleModel & {
    schema: {
        id:    "resource-001"
        name:  "My Simple Resource"
        value: 42
    }
}
```

The `&` operator unifies your values against the model's schema constraints. CUE validates type compatibility at parse time.

## Referencing Models

Import models from the schema repository:

```cue
import "pudl.schemas/pudl/model/examples"

api_endpoint: examples.#HTTPEndpointModel & {
    schema: {
        method: "GET"
        url:    "https://api.example.com/data"
    }
}
```

## Socket Wiring

Connect definitions by referencing other definitions' outputs:

```cue
package definitions

import "pudl.schemas/pudl/model/examples"

prod_vpc: {
    _model: "examples.#VPCModel"
    outputs: {
        vpc_id: "vpc-abc123"
    }
}

prod_instance: examples.#EC2InstanceModel & {
    schema: {
        InstanceId:   "i-pending"
        InstanceType: "t3.micro"
        ImageId:      "ami-12345"
        State: {Name: "pending", Code: 0}
        VpcId: prod_vpc.outputs.vpc_id  // wired to VPC output
    }
}
```

CUE validates the type of `prod_vpc.outputs.vpc_id` matches what the EC2 model expects for `VpcId`.

## Marker-Based Definitions

For definitions without a model type in the CUE registry, use a `_model` marker:

```cue
my_resource: {
    _model: "examples.#SomeModel"
    outputs: {
        resource_id: "res-123"
    }
}
```

The `_model` field is used for discovery but does not participate in CUE validation.

## Vault References (Phase 5 Preview)

Secret values can be marked with vault references for future resolution:

```cue
prod_db: examples.#DatabaseModel & {
    schema: {
        host:     "db.example.com"
        password: vault."prod/db/password"  // resolved at execution time
    }
}
```

Vault resolution is not yet implemented. Values are stored as markers.

## CLI Commands

```bash
pudl definition list              # List all definitions
pudl definition show <name>       # Show definition details
pudl definition validate          # Validate all definitions
pudl definition validate <name>   # Validate one definition
pudl definition graph             # Show dependency graph
pudl repo validate                # Validate everything
```

## Conventions

- One concern per file — group related definitions together.
- Name definitions with lowercase snake_case.
- Prefix environment-specific definitions: `prod_`, `staging_`, `dev_`.
- Keep socket wiring explicit — avoid deeply nested cross-references.
