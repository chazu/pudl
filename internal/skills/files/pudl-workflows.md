# Composing PUDL Workflows

Workflows are CUE files describing DAGs of method executions. Steps run concurrently when they have no data dependencies.

## Workflow Structure

Workflows live in `workflows/*.cue` in the schema directory:

```cue
package workflows

#DeployWorkflow: {
    name: "deploy-stack"
    description: "Deploy full application stack"

    steps: {
        create_vpc: {
            definition: "prod_vpc"
            method:     "create"
            timeout:    "5m"
        }
        create_subnet: {
            definition: "prod_subnet"
            method:     "create"
            inputs: {
                vpc_id: steps.create_vpc.outputs.VpcId
            }
            timeout: "2m"
        }
        launch_instance: {
            definition: "prod_instance"
            method:     "create"
            inputs: {
                SubnetId: steps.create_subnet.outputs.SubnetId
            }
            timeout:   "3m"
            retries:   2
            condition: "steps.create_subnet.status == 'success'"
        }
    }
}
```

## Step Fields

| Field | Required | Description |
|-------|----------|-------------|
| `definition` | Yes | Definition name to execute against |
| `method` | Yes | Method name to run |
| `inputs` | No | Override method inputs; can reference `steps.<name>.outputs.<field>` |
| `condition` | No | Expression that must be true for step to run |
| `timeout` | No | Per-step timeout (default: method's timeout) |
| `retries` | No | Number of retry attempts on failure |

## Dependency Resolution

Dependencies are inferred from CUE field references between steps:
- `steps.create_vpc.outputs.VpcId` creates an edge: `create_subnet` depends on `create_vpc`
- Steps with no references to other steps run concurrently

## CLI Commands

```bash
# Run a workflow
pudl workflow run deploy-stack

# Validate workflow DAG (check for cycles, missing definitions)
pudl workflow validate deploy-stack

# List available workflows
pudl workflow list

# Show workflow details and DAG structure
pudl workflow show deploy-stack

# View execution history
pudl workflow history deploy-stack
```

## Run Manifests

Each workflow run produces a manifest at `.pudl/data/.runs/<workflow>/<run-id>.json` recording:
- Step execution order and timing
- Per-step status (success/failed/skipped)
- Artifact references for step outputs
- Overall workflow status

## Best Practices

- Keep workflows focused on one logical operation (deploy, teardown, migrate)
- Use conditions for optional steps rather than separate workflows
- Set appropriate timeouts per step
- Use retries for steps that may fail transiently (API calls, provisioning)
- Reference step outputs via `steps.<name>.outputs.<field>` for data flow
