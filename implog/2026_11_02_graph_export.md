# pudl graph export: the web explorer reads real catalog data (pudl-jv2)

`pudl graph export [-o file]` writes the current resource graph in the
canonical JSON shape the web explorer loads with `?graph=<url>`. Design and
format: [docs/frontend.md](../docs/frontend.md#from-a-pudl-workspace-pudl-graph-export).

## Where the knowledge lives

Following "knowledge lives in CUE", the Go code is generic. The packs say
what relationships exist and how resources are labelled.

- **Edges** are Datalog rules for the relation
  `graph_edge(type, source, target, basis)`, joined over facts the packs
  already project. `source` and `target` are a `resource_id` or a reference
  value. The exporter resolves references through identity values and
  `_pudl.graph.refs`. An unresolved reference becomes `ref:<value>`; an
  ambiguous one is reported, never guessed. Null targets (absent optional
  fields) mean "no edge". Each rule is evaluated alone, so each relationship
  names its rule.
- **Labels** come from a new strictly decoded `_pudl.graph` block: name,
  account, region and zone, plus `refs`. Values are field paths or
  `{path, trim_prefix?, last_segment?}`, and the block is inherited along
  `base_schema`.

## Public API

- `internal/graphspec`: `Parse(raw) (*Spec, error)`, `Spec{Name, Account,
  Region, Zone *Value; Refs []Value}`, `Value.Eval`, `Eval`,
  `Spec.RefValues`, `Scalar`. Pure.
- `internal/validator`: `SchemaMetadata.Graph`, `.GraphError`; decode errors
  are schema load errors.
- `internal/graphexport`: `Build(ctx, Input) (*Graph, error)`,
  `EvaluateEdgeRules`, types `Graph/Meta/Resource/Relationship/Evidence`,
  constants `EdgeRelation`, `RefPrefix`, `InternalTypes`, `Bases`.
- `cmd/graph.go`: `pudl graph`, `pudl graph export -o`.

## Pack content

- AWS: `aws/graph.cue` (`_awsGraph`) plus `graph` on every type;
  `pudl/rules/aws_graph.cue` covers in_subnet, protected_by, in_vpc,
  attached_to and encrypted_by for instances, volumes and RDS.
- GCP: `gcp/graph.cue` (`_gcpGraph`, `_gcpComputeGraph` with the trimmed
  selfLink ref) plus `graph` on every type; `pudl/rules/gcp_graph.cue` covers
  in_network, nat_ip, in_subnet (GKE), encrypted_by, private_ip_in,
  private_zone_for and subscribed_to. New fact `gcp_subscription_topic`.
- Excluded from the graph: `system_model`, `pudl.observe`,
  `pudl.observe_snapshot` (pudl's own records).

## Frontend

- Owners are found by type and native ID (`GraphIndex.byNative`), not the
  demo's ID convention. `Provider` is an open string.
- New resource types (EBS, GKE, route, address, DNS zone, GCP KMS) and edge
  types (`nat_ip`, `private_zone_for`). The network view places GKE clusters,
  routes and addresses, and places a GCP network by its project attribute
  when no project record exists.
- The inspector shows the schema and the `pudl show <entry>` handle. Producer
  warnings appear in the status bar.

## Verification

- Go: `internal/graphspec` and `internal/graphexport` unit tests;
  `TestGraphExportAWSPack` and `TestGraphExportGCPPack` run the fixture
  collectors and export, with no warnings, resolved selfLink references, one
  explicit unresolved GKE subnet, and no internal types. The JSON contract,
  help golden and `docs/cli-reference.md` are updated. `go test ./...`
  passes.
- Web: 42 vitest tests; both packs' fixtures (86 resources, 63
  relationships) checked in the browser in the network, inventory and
  security views and the inspector.

## Known gaps

- No IAM trust or policy facts and no security-group rule facts, so the
  security view has few edges on pack data.
- The export is a file snapshot; nothing serves the catalog live yet.
