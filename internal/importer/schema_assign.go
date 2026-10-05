package importer

import (
	"fmt"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// fallbackItemSchema is the catchall assigned when inference itself fails.
const fallbackItemSchema = "pudl/core.#Item"

// schemaAssignment is the schema chosen for one record and how it was chosen.
type schemaAssignment struct {
	Schema     string
	Confidence float64
	// Validation is set when the schema came from --schema and was checked by
	// the chain validator. Nil for inferred assignments.
	Validation *validator.ValidationResult
}

// assignSchema chooses the schema for one decoded record.
//
// Without a manual schema the record is inferred. With one, the record is
// validated against it through the chain (intended → base → catchall), so a
// user-supplied schema is honored when the data satisfies it and the record
// still lands somewhere when it does not — data is never rejected.
//
// A manual schema the validator has not loaded (e.g. a module-versioned CUE
// ref resolved later by `pudl reclassify`) cannot be checked; it is recorded as
// given, at reduced confidence, rather than silently replaced by a catchall.
func (e *EnhancedImporter) assignSchema(data interface{}, opts ImportOptions, hints inference.InferenceHints) (schemaAssignment, error) {
	if opts.ManualSchema == "" {
		result, err := e.inferrer.Infer(data, hints)
		if err != nil {
			return schemaAssignment{}, fmt.Errorf("failed to infer schema: %w", err)
		}
		return schemaAssignment{Schema: schemaname.Normalize(result.Schema), Confidence: result.Confidence}, nil
	}

	chain, err := e.chainValidator(opts)
	if err != nil {
		return schemaAssignment{}, err
	}
	intended := schemaname.Normalize(opts.ManualSchema)
	if !chain.HasSchema(intended) {
		return schemaAssignment{Schema: intended, Confidence: 0.5}, nil
	}

	result, err := chain.ValidateChain(data, opts.ManualSchema)
	if err != nil {
		return schemaAssignment{}, fmt.Errorf("failed to validate against %s: %w", opts.ManualSchema, err)
	}
	confidence := 1.0
	if !result.Valid {
		confidence = 0.5
	}
	return schemaAssignment{
		Schema:     schemaname.Normalize(result.AssignedSchema),
		Confidence: confidence,
		Validation: result,
	}, nil
}

// assignItemSchema assigns a schema to one collection item. Inference failures
// fall back to the catchall so one odd record cannot abort a collection.
func (e *EnhancedImporter) assignItemSchema(itemData interface{}, opts ImportOptions) (string, float64) {
	assigned, err := e.assignSchema(itemData, opts, inference.InferenceHints{
		Format:         "json",
		CollectionType: "item",
	})
	if err != nil {
		return fallbackItemSchema, 0.5
	}
	return assigned.Schema, assigned.Confidence
}

// chainValidator returns the validator for manual-schema imports: the caller's
// when supplied, otherwise one built once over the importer's schema paths.
func (e *EnhancedImporter) chainValidator(opts ImportOptions) (*validator.ChainValidator, error) {
	if opts.ChainValidator != nil {
		return opts.ChainValidator, nil
	}
	if e.chain == nil {
		chain, err := validator.NewChainValidator(e.schemaPaths...)
		if err != nil {
			return nil, fmt.Errorf("failed to create chain validator: %w", err)
		}
		e.chain = chain
	}
	return e.chain, nil
}
