package datalog

import "github.com/chazu/pudl/internal/database"

// builtinEDBTables maps built-in EDB relation names to the SQL table/view that
// backs them. Body atoms referencing these relations compile to a join against
// the backing table (native columns) instead of the facts table. The relation
// names are reserved by the database package (see database.IsReservedRelation);
// a sync test asserts the two stay aligned.
var builtinEDBTables = map[string]string{
	database.CatalogEntryRelation: database.CatalogEntryView,
	database.FactScoredRelation:   database.FactScoredView,
}

// withBuiltinEDB returns a copy of overrides with the built-in EDB table
// mappings merged in. Caller-supplied overrides (e.g. recursive _delta_
// tables) take precedence, though built-in relations are never derived so no
// key collision occurs in practice.
func withBuiltinEDB(overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(overrides)+len(builtinEDBTables))
	for rel, table := range builtinEDBTables {
		merged[rel] = table
	}
	for rel, table := range overrides {
		merged[rel] = table
	}
	return merged
}
