package cmd

import (
	"fmt"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/schemaname"
)

type inventoryNamespace func(string) (string, error)

// inventoryIdentityPolicy resolves both routing tags and CUE names to their
// identity family. A coincidental resource_type shared by distinct families is
// ambiguous, so callers must name the intended schema explicitly.
func inventoryIdentityPolicy() (identityResolver, inventoryNamespace, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return nil, nil, err
	}
	schemas, err := inference.Shared(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return nil, nil, err
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
	return fields, resolve, nil
}

func normalizeInventoryRecord(record map[string]any, namespace inventoryNamespace) (map[string]any, error) {
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
