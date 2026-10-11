package mubridge

import (
	"fmt"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// observeRoute decides each observed record's schema and redacts it.
type observeRoute struct {
	mismatches    *int
	allowFallback bool
	manual        string
	chain         *validator.ChainValidator
	redactor      *redact.Registry
	// Redacted counts sensitive values replaced across the observation.
	redacted *int
}

func newObserveRoute(in ObserveIngest) (observeRoute, error) {
	route := observeRoute{allowFallback: in.AllowSchemaFallback, redactor: in.Redactor, redacted: new(int), mismatches: new(int)}
	if route.redactor == nil && in.Inferrer != nil {
		route.redactor = redact.NewRegistry(in.Inferrer)
	}
	if in.ManualSchema != "" {
		if in.Chain == nil {
			return observeRoute{}, fmt.Errorf("observe ingest: a manual schema needs a chain validator")
		}
		route.manual = schemaname.Normalize(in.ManualSchema)
		route.chain = in.Chain
		if !route.chain.HasSchema(route.manual) {
			return observeRoute{}, fmt.Errorf("schema %s is not loaded", route.manual)
		}
	}
	return route, nil
}

// resolve returns the record's schema: the manual schema validated through
// its chain (falling back to base or catchall exactly as `import --schema`
// does), or the _schema-based routing.
func (r observeRoute) resolve(record map[string]any, graph *inference.InheritanceGraph, inferrer *inference.SchemaInferrer, mappings map[string]string) (string, error) {
	if r.manual == "" {
		return resolveObserveSchemaWithMappings(record, graph, inferrer, mappings), nil
	}
	if !r.chain.HasSchema(r.manual) {
		return "", fmt.Errorf("schema %s is not loaded", r.manual)
	}
	result, err := r.chain.ValidateChain(record, r.manual)
	if err != nil {
		return "", fmt.Errorf("validate against %s: %w", r.manual, err)
	}
	if !result.Valid && !r.allowFallback {
		return "", fmt.Errorf("observed record does not satisfy requested schema %s", r.manual)
	}
	if !result.Valid {
		*r.mismatches++
	}
	return schemaname.Normalize(result.AssignedSchema), nil
}

// redact applies the sensitive paths of the intended and assigned schemas. A
// broken declaration on either is an error: the record is not stored.
func (r observeRoute) redact(record map[string]any, schema string) error {
	paths, err := r.redactor.For(r.manual, schema)
	if err != nil {
		return err
	}
	*r.redacted += redact.Apply(record, paths)
	return nil
}
