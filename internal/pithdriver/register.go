package pithdriver

import (
	"github.com/chazu/pith"

	"pudl/internal/database"
	"pudl/internal/schema"
)

// Register adds all pudl driver words to a pith VM.
// Registers catalog/*, fact/*, and schema/* vocabularies.
func Register(vm *pith.VM, db *database.CatalogDB, mgr *schema.Manager) {
	if db != nil {
		vm.RegisterDriver("catalog", catalogWords(db))
		vm.RegisterDriver("fact", factWords(db))
	}
	if mgr != nil {
		vm.RegisterDriver("schema", schemaWords(mgr))
	}
}
