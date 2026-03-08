# Writing PUDL Definitions

Definitions are named instances of models with concrete configuration. They live in `definitions/*.cue` in the schema directory.

## Definition Structure

A definition unifies against a model type and provides concrete values for sockets and configuration:

```cue
package definitions

import "pudl.schemas/pudl/model/examples"

// A named instance of the EC2 model
prod_instance: examples.#EC2InstanceModel & {
    // Socket bindings — concrete values for model inputs
    sockets: {
        vpc_id: {
            value: "vpc-abc123"
        }
        subnet_id: {
            value: "subnet-def456"
        }
    }

    // Override method inputs if needed
    methods: {
        create: {
            inputs: {
                InstanceType: "t3.medium"
                ImageId:      "ami-12345678"
            }
        }
    }
}
```

## Socket Wiring

Definitions can reference each other's output sockets, creating a dependency graph:

```cue
package definitions

import "pudl.schemas/pudl/model/examples"

// VPC definition (provides vpc_id output)
my_vpc: examples.#VPCModel & {
    methods: create: inputs: CidrBlock: "10.0.0.0/16"
}

// Instance depends on VPC (wires vpc_id input from VPC output)
my_instance: examples.#EC2InstanceModel & {
    sockets: vpc_id: value: my_vpc.sockets.vpc_id.value
}
```

## Vault References

Use `vault://` references for secrets — resolved at execution time, never stored in artifacts:

```cue
sockets: {
    api_key: {
        value: "vault://aws/access_key"
    }
}
```

## Validation

Run `pudl definition validate` to validate all definitions against their model schemas. This checks CUE type constraints and socket wiring consistency.

Run `pudl definition graph` to visualize the dependency DAG from socket wiring.

## Best Practices

- One definition per logical resource instance
- Use descriptive names matching your infrastructure (e.g., `prod_db`, `staging_api`)
- Wire sockets between definitions rather than hardcoding values
- Use vault references for all credentials
- Keep definitions in separate files by environment or service group
