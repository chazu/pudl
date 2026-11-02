# Model load rejects desired state on sensitive paths — pudl-o5i

`docs/design/2026-10-08-gcp-cataloging-ux.md` §6.2 documented a limit:
redacted observed values never equal plaintext desired values, so desired
state must not be declared on sensitive paths. Nothing enforced it. A desired
value at a sensitive path is authored plaintext that would be copied into
drift findings and run reports as the expected value, and it drifts forever
because the observed value is `[REDACTED]`.

The ticket notes add a second load-time check (from `pudl-xf5.8.1`): a
`#CommandObserve` `populate.schema` that names a policy was rejected only per
record during ingest, after the commands had run.

## Public API

```go
// internal/systemmodel (schema_contract.go)
// CheckSchemaContracts rejects a model whose authored values contradict the
// loaded schemas its records are routed to. src is satisfied by
// *inference.SchemaInferrer.
func CheckSchemaContracts(m *SystemModel, src redact.Source) error
```

`cmd/model_validate.go` adds `checkModelSchemas(m)`, which loads the
workspace's schemas (`inference.Shared(effectiveSchemaPaths(cfg)...)`) and
calls `CheckSchemaContracts`. It is called from:

- `resolveRunModel` (`cmd/run_execute.go`, three lines): every `pudl run`
  (named, run-set member, or ad-hoc) after elaboration and before any command
  runs;
- `validateModelTemplate`: `pudl model validate` reports a violation as a
  problem, so the model fails validation.

## Behavior

- **populate.schema naming a policy.** The error wraps
  `*validator.PolicySchemaError`, so it names the policy and its nearest type
  (`validator.NearestType`): `model "m": populate.schema: x.#P is a policy
  schema, not a type: name its type x.#T instead; ...`.
- **Desired at a sensitive path.** Each desired record's sensitive paths are
  `redact.Registry.For(populate.schema, _schema tag, schemas whose
  resource_type equals the tag...)`. `For` already takes the union along
  `base_schema`. `populate.schema` is included because ingest redacts with
  both the manual and the assigned schema (`observe.observeRoute.redact`).
  The tag is resolved both as a schema name and as a resource_type, because
  desired records commonly carry the resource-type tag (`gcp.bucket`), not
  the canonical name. Any value at a path, including `null` and values under a
  wildcard, is a violation. The error names the model, the record
  (`desired[i] (<_schema>)`) and the path, and says to remove it from desired.
- **Broken declarations fail closed.** A `SensitiveError`, an unparsable
  sensitive path, or a sensitive path overlapping an identity field on any
  schema in the union is a load error:
  `model "m": desired[0] (x.#T): cannot check sensitive paths: ...`.
- **Unloaded schemas do not fail the load.** An unknown `_schema` tag or
  `populate.schema` contributes no paths; ingest still reports an unloaded
  `populate.schema` as before. A model with neither `desired` nor
  `populate.schema` does not load schemas at all. A failure to load the schema
  repository is an error, as it already was for these models at drift
  (`inventoryIdentityPolicy`) and ingest.
- **Bound templates.** `pudl model validate` checks a template with inputs
  only structurally, as before; its concrete values are checked by `pudl run`
  after elaboration.
- **Sealed references.** The ticket asked the error to point at the
  sealed-reference channel. Sealed values were retired on 2026-11-01
  (`systemmodel/retired.go`), so the message only says to remove the value.

## Tests

- `internal/systemmodel/schema_contract_test.go`: desired at a sensitive
  path; at an inherited path reached through a resource_type tag; a broken
  declaration; `populate.schema` naming a policy (`errors.As`
  `*PolicySchemaError`, nearest type); unaffected models (other paths, a schema
  without sensitive fields, unloaded schemas).
- `cmd/model_validate_test.go`: `pudl model validate` on a real schema
  repository reports the violation as a problem.

## Line delta against sre-focus

- Production: +85 / −1 (net +84): `schema_contract.go` +59,
  `cmd/model_validate.go` +22 net, `cmd/run_execute.go` +3.
- Tests: +117 / −0.
- Docs: plan entry, §6.2 note in the cataloging UX design, this implog.

## Verification

- `CGO_ENABLED=0 go test ./...`: all packages pass.
