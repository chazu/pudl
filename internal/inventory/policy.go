package inventory

import (
	"fmt"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/schemaname"
)

// Namespace maps a record's `_schema` (a routing tag or a CUE name) to its
// identity family.
type Namespace func(string) (string, error)

// Policy is how records are identified: the identity fields a schema
// declares, and the identity family a schema name belongs to. The zero Policy
// identifies by the fallback fields and leaves `_schema` as written.
type Policy struct {
	Identity  acute.IdentityResolver
	Namespace Namespace
}

// LoadPolicy resolves both routing tags and CUE names to their identity
// family over the schemas in schemaPaths. A coincidental resource_type shared
// by distinct families is ambiguous, so callers must name the intended schema
// explicitly.
func LoadPolicy(schemaPaths []string) (Policy, error) {
	schemas, err := inference.Shared(schemaPaths...)
	if err != nil {
		return Policy{}, err
	}
	byType := map[string][]string{}
	for _, name := range schemas.GetAvailableSchemas() {
		if meta, ok := schemas.GetSchemaMetadata(name); ok && meta.ResourceType != "" {
			byType[meta.ResourceType] = append(byType[meta.ResourceType], name)
		}
	}
	resolve := func(name string) (string, error) {
		canonical := schemaname.Normalize(name)
		if _, ok := schemas.GetSchemaMetadata(canonical); ok {
			return schemas.GetInheritanceGraph().IdentityRoot(canonical), nil
		}
		root := ""
		for _, candidate := range byType[name] {
			next := schemas.GetInheritanceGraph().IdentityRoot(candidate)
			if root != "" && root != next {
				return "", fmt.Errorf("resource type %q has multiple identity families; use an explicit schema name", name)
			}
			root = next
		}
		if root == "" {
			return name, nil
		}
		return root, nil
	}
	fields := func(name string) []string {
		resolved, err := resolve(name)
		if err != nil {
			return nil
		}
		if meta, ok := schemas.GetSchemaMetadata(resolved); ok {
			return meta.IdentityFields
		}
		return nil
	}
	return Policy{Identity: fields, Namespace: resolve}, nil
}

// NormalizeRecord returns a copy of record whose `_schema` names its identity
// family. A nil namespace returns record unchanged.
func NormalizeRecord(record map[string]any, namespace Namespace) (map[string]any, error) {
	if namespace == nil {
		return record, nil
	}
	name, _ := record["_schema"].(string)
	canonical, err := namespace(name)
	if err != nil {
		return nil, err
	}
	copy := make(map[string]any, len(record))
	for key, value := range record {
		copy[key] = value
	}
	copy["_schema"] = canonical
	return copy, nil
}
