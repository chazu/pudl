package observe

import (
	"fmt"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/secretscan"
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
	// scanner warns of likely secrets outside sensitive coverage; secrets
	// collects the warnings. Neither changes a record.
	scanner *secretscan.Scanner
	secrets *secretscan.Tally
}

func newObserveRoute(in ObserveIngest) (observeRoute, error) {
	route := observeRoute{allowFallback: in.AllowSchemaFallback, redactor: in.Redactor, redacted: new(int), mismatches: new(int)}
	var src redact.Source
	if in.Inferrer != nil {
		src = in.Inferrer
	}
	if route.redactor == nil && src != nil {
		route.redactor = redact.NewRegistry(src)
	}
	route.scanner, route.secrets = secretscan.New(src, route.redactor), &secretscan.Tally{}
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
// its chain (fallback requires explicit opt-in), or _schema-based routing.
func (r observeRoute) resolve(record map[string]any, graph *inference.InheritanceGraph, inferrer *inference.SchemaInferrer) (string, error) {
	if r.manual == "" {
		// A record that claims a type by @pudl(match) is assigned that type or
		// rejected, never routed to the generic observe schema.
		if inferrer != nil && inferrer.Claims(record) {
			result, err := inferrer.Infer(record, inference.InferenceHints{CollectionType: "item"})
			if err != nil {
				return "", err
			}
			return schemaname.Normalize(result.Schema), nil
		}
		return resolveObserveSchema(record, graph, inferrer), nil
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

// scanSecrets notes likely secrets in an already redacted record, by record
// index, path and kind.
func (r observeRoute) scanSecrets(record map[string]any, index int, schema string) {
	r.secrets.Note(index, schema, r.scanner.Scan(record, r.manual, schema))
}
