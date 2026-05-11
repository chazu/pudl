package pithdriver

import (
	"github.com/chazu/pith"

	"pudl/internal/schema"
)

// schemaWords returns the schema driver vocabulary.
func schemaWords(mgr *schema.Manager) map[string]pith.Word {
	return map[string]pith.Word{
		"list": wordSchemaList(mgr),
	}
}

// schema/list ( -- [schemas] )
func wordSchemaList(mgr *schema.Manager) pith.Word {
	return func(vm *pith.VM) error {
		schemas, err := mgr.ListSchemas()
		if err != nil {
			return err
		}
		result := make(map[string]any)
		for pkg, infos := range schemas {
			items, err := structsToMaps(infos)
			if err != nil {
				return err
			}
			result[pkg] = items
		}
		vm.Push(result)
		return nil
	}
}
