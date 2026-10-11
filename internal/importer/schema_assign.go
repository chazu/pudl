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
	Reason     string
	SourcePath string
	Trace      *inference.InferenceTrace
	// Validation is set when the schema came from --schema and was checked by
	// the chain validator. Nil for inferred assignments.
	Validation *validator.ValidationResult
	// Validated reports that the record satisfied the explicit --schema itself
	// (not a fallback). Only such records may move an existing entry.
	Validated bool
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
		var result *inference.InferenceResult
		var err error
		if opts.Explain {
			result, err = e.inferrer.InferWithTrace(data, hints)
		} else {
			result, err = e.inferrer.Infer(data, hints)
		}
		if err != nil {
			return schemaAssignment{}, fmt.Errorf("failed to infer schema: %w", err)
		}
		source, _ := e.inferrer.SchemaSource(result.Schema)
		return schemaAssignment{Schema: schemaname.Normalize(result.Schema), Confidence: result.Confidence, Reason: result.Reason, SourcePath: source, Trace: result.Trace}, nil
	}

	chain, err := e.chainValidator(opts)
	if err != nil {
		return schemaAssignment{}, err
	}
	intended := schemaname.Normalize(opts.ManualSchema)
	if !chain.HasSchema(intended) {
		if !opts.AllowSchemaFallback {
			return schemaAssignment{}, fmt.Errorf("requested schema %s is unavailable; load it before importing or explicitly allow schema fallback", intended)
		}
		assigned := schemaAssignment{Schema: intended, Confidence: 0.5, Reason: "explicit schema unavailable; assignment retained without validation"}
		if opts.Explain {
			assigned.Trace = &inference.InferenceTrace{Selected: intended, Reason: assigned.Reason, ScoreKind: "heuristic score, not a calibrated probability", Attempts: []inference.CandidateAttempt{{Schema: intended, Reason: "schema unavailable"}}}
		}
		return assigned, nil
	}

	result, err := chain.ValidateChain(data, opts.ManualSchema)
	if err != nil {
		return schemaAssignment{}, fmt.Errorf("failed to validate against %s: %w", opts.ManualSchema, err)
	}
	if !result.Valid && !opts.AllowSchemaFallback {
		return schemaAssignment{}, fmt.Errorf("record does not satisfy requested schema %s; inspect with --dry-run --allow-schema-fallback --explain", intended)
	}
	confidence := 1.0
	if !result.Valid {
		confidence = 0.5
	}
	reason := "explicit schema validated"
	if !result.Valid {
		reason = result.FallbackReason
	}
	source, _ := e.inferrer.SchemaSource(result.AssignedSchema)
	assigned := schemaAssignment{
		Reason: reason, SourcePath: source,
		Schema:     schemaname.Normalize(result.AssignedSchema),
		Confidence: confidence,
		Validation: result,
		Validated:  result.Valid,
	}
	if opts.Explain {
		assigned.Trace = &inference.InferenceTrace{Selected: assigned.Schema, Reason: reason, Fallback: !result.Valid, ScoreKind: "heuristic score, not a calibrated probability", Attempts: []inference.CandidateAttempt{}}
		for _, attempt := range result.ChainAttempts {
			source, shadowed := e.inferrer.SchemaSource(attempt.SchemaName)
			item := inference.CandidateAttempt{Schema: attempt.SchemaName, Reason: attempt.Reason, Matched: attempt.Success, SourcePath: source, ShadowedPaths: shadowed}
			for i, err := range attempt.Errors {
				if i == 16 {
					break
				}
				item.FailurePaths = append(item.FailurePaths, err.Path)
			}
			assigned.Trace.Attempts = append(assigned.Trace.Attempts, item)
		}
	}
	return assigned, nil
}

// assignItemSchema assigns a schema to one collection item. Inference failures
// fall back to the catchall so one odd record cannot abort a collection.

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

// assignItemSchemaDetailed retains diagnostics for prepared collection records.
func (e *EnhancedImporter) assignItemSchemaDetailed(data any, opts ImportOptions) (schemaAssignment, error) {
	assigned, err := e.assignSchema(data, opts, inference.InferenceHints{Format: "json", CollectionType: "item"})
	if err == nil {
		return assigned, nil
	}
	if opts.ManualSchema != "" && !opts.AllowSchemaFallback {
		return schemaAssignment{}, err
	}
	// Inference failure retains the existing collection catchall behavior.
	assigned = schemaAssignment{Schema: fallbackItemSchema, Confidence: 0.5, Reason: "classification failed; collection item assigned to catchall"}
	if opts.Explain {
		assigned.Trace = &inference.InferenceTrace{Selected: assigned.Schema, Reason: assigned.Reason, Fallback: true, ScoreKind: "heuristic score, not a calibrated probability", Attempts: []inference.CandidateAttempt{}}
	}
	return assigned, nil
}

// enrichAssignment persists the original reason, and an opt-in diagnostic trace.
func enrichAssignment(info *SchemaInfo, result *ImportResult, assigned schemaAssignment) {
	if info != nil {
		info.AssignmentReason = assigned.Reason
		info.SourcePath = assigned.SourcePath
		info.Explanation = assigned.Trace
	}
	if result != nil {
		result.Explanation = assigned.Trace
	}
}

func (e *EnhancedImporter) validateExplicitSchema(opts ImportOptions) error {
	if opts.ManualSchema == "" || opts.AllowSchemaFallback {
		return nil
	}
	chain, err := e.chainValidator(opts)
	if err != nil {
		return err
	}
	if !chain.HasSchema(schemaname.Normalize(opts.ManualSchema)) {
		return fmt.Errorf("requested schema %s is unavailable", opts.ManualSchema)
	}
	return nil
}
