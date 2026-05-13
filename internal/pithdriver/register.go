package pithdriver

import (
	"github.com/chazu/pith"

	"pudl/internal/database"
	"pudl/internal/inference"
	"pudl/internal/schema"
)

// Register adds all pudl driver words to a pith VM.
// Registers catalog/*, fact/*, schema/*, and drift/* vocabularies.
// The inferrer parameter is optional; when nil, schema/match and
// schema/infer are not registered.
func Register(vm *pith.VM, db *database.CatalogDB, mgr *schema.Manager, inferrer *inference.SchemaInferrer) {
	if db != nil {
		vm.RegisterDriver("catalog", catalogWords(db))
		vm.RegisterDriver("fact", factWords(db))
	}
	if mgr != nil {
		words := schemaWords(mgr)
		if inferrer != nil {
			for k, v := range schemaInferWords(inferrer) {
				words[k] = v
			}
		}
		vm.RegisterDriver("schema", words)
	}
	vm.RegisterDriver("drift", driftWords())
}
