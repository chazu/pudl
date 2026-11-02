package systemmodel

import (
	"fmt"

	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// CheckSchemaContracts rejects a model whose authored values contradict the
// loaded schemas its records are routed to:
//
//   - populate.schema must name a type, not a policy. Ingest rejects a policy
//     per record, but only after the model's commands have run.
//   - desired must set no value at a sensitive path of its record's schema
//     (the union along base_schema, plus populate.schema's). Observed values
//     there are stored as [REDACTED], so a plaintext desired value never
//     matches, and it would be copied into drift findings and run reports.
//
// A broken sensitive declaration fails closed. A schema that is not loaded
// constrains nothing here; ingest reports it when records are routed to it.
// A desired record's `_schema` tag is resolved both as a schema name and as a
// resource_type, and every match contributes its sensitive paths.
func CheckSchemaContracts(m *SystemModel, src redact.Source) error {
	if m == nil || src == nil {
		return nil
	}
	manual := schemaname.Normalize(m.Populate.Schema)
	if meta, ok := src.GetSchemaMetadata(manual); ok && meta.IsPolicy() {
		policyErr := &validator.PolicySchemaError{Policy: manual, Type: validator.NearestType(manual, src.GetSchemaMetadata)}
		return fmt.Errorf("model %q: populate.schema: %w", m.Name, policyErr)
	}
	if len(m.Desired) == 0 {
		return nil
	}
	byType := map[string][]string{}
	for _, name := range src.GetAvailableSchemas() {
		if meta, ok := src.GetSchemaMetadata(name); ok && meta.ResourceType != "" {
			byType[meta.ResourceType] = append(byType[meta.ResourceType], name)
		}
	}
	sensitive := redact.NewRegistry(src)
	for i, record := range m.Desired {
		tag, _ := record["_schema"].(string)
		resource := fmt.Sprintf("desired[%d] (%s)", i, tag)
		paths, err := sensitive.For(append([]string{m.Populate.Schema, tag}, byType[tag]...)...)
		if err != nil {
			return fmt.Errorf("model %q: %s: cannot check sensitive paths: %w", m.Name, resource, err)
		}
		for _, path := range paths {
			if len(path.LookupAll(record)) > 0 {
				return fmt.Errorf("model %q: %s sets sensitive path %q; observed values there are stored as %s, so a desired value can never match and would be copied into reports as plaintext — remove it from desired",
					m.Name, resource, path.String(), redact.Placeholder)
			}
		}
	}
	return nil
}
