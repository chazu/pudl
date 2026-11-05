# Dependencies and impact

pudl answers two dependency questions with Datalog:

- **Between models**: which models must observe before this one, and which
  models a change to this one affects.
- **Between resources**: what cataloged resources depend on this VPC, network,
  security group or key. The content packs ship these rules.

Both are read-only. A dependency orders checks and answers queries; it passes
no values between models.

## Declare model dependencies

```cue
#AppChecks: sm.#SystemModel & {
	name:       "app-checks"
	depends_on: ["network", "gke-clusters"]   // other models' name fields
	checks: [...]
}
```

Every `pudl check` records each named model's declared edges as
`model_depends_on(from, to)` facts. It adds new edges and invalidates removed
ones, so fact history shows when a dependency appeared or ended.
`pudl model deps` records the edges of every registered model without running
any, and prints the graph. Run it before asking impact questions about models
that have never been checked.

A dependency that names no registered model is still recorded, with a warning,
so forward references work. A model that names itself is ignored with a
warning.

## How dependencies affect a check

`pudl check a b c` runs each named model after the named models it depends
on. A dependency that is not named adds no ordering and is never run. When a
named producer fails, its dependents are recorded `blocked`.

`pudl check MODEL --check-upstream` warns when any model MODEL transitively
depends on is recorded `drifted` or `failed`. The warning is advisory: it
covers only edges that have been recorded.

## Query the graph

Built-in rules read `model_depends_on`:

| Relation | Arguments | Answers |
|----------|-----------|---------|
| `model_depends_on` | `from`, `to` | Direct dependencies (facts) |
| `depends_transitive` | `from`, `to` | Direct and transitive dependencies |
| `impacted_by` | `changed`, `impacted` | Models affected when `changed` changes |
| `cyclic` | `model` | Models that depend on themselves |

```bash
pudl query depends_transitive from=app-checks   # what app-checks depends on
pudl query impacted_by changed=network          # what a network change affects
pudl query cyclic                               # dependency cycles
pudl query --topo model_depends_on              # a run order, dependencies first
```

`--topo` errors on a cycle. `pudl query --list` shows every relation and its
argument keys.

## Resource impact (content packs)

The packs project relationship facts from their schemas and ship rules over
them:

| Pack | Relation | Answers |
|------|----------|---------|
| `gcp` | `gcp_depends_on_network(network, resource, kind)` | Resources that use a VPC network: subnets, firewalls, routes, routers, GKE clusters, Cloud SQL private IPs, private DNS zones |
| `aws` | `aws_uses(resource, kind, uses)` | One direct dependency, such as instance → subnet |
| `aws` | `aws_depends_on(resource, kind, on)` | The transitive closure of `aws_uses` |
| `aws` | `aws_depends_on_vpc(vpc, resource, kind)` | Everything that depends on a VPC |

```bash
pudl query gcp_depends_on_network network=projects/example-project/global/networks/prod-vpc
pudl query aws_depends_on on=sg-0aa00000000000003
```

These read latest-known facts, so run the collectors first. See the
[GCP](../packs/gcp/README.md#impact-what-depends-on-this-network) and
[AWS](../packs/aws/README.md#relationships-and-impact) pack READMEs. Write
your own impact rules the same way: project the references with
`_pudl.facts` ([projection](projection.md)) and join them in a recursive rule
([Datalog](datalog.md)).

Value-equality dependency guessing (`pudl model deps --derive`) and value
bindings between models were retired; see
[retired commands](retired-commands.md).
