package database

// Built-in EDB relations exposed to the Datalog engine. These relation names
// are backed by catalog tables/views rather than the facts table, so they are
// reserved: facts may not be asserted under these names (see AddFact). The
// datalog package's override map must reference the same names (a sync test
// enforces this).

// CatalogEntryRelation is the Datalog relation name for catalog entries.
const CatalogEntryRelation = "catalog_entry"

// CatalogEntryView is the SQL view exposing catalog_entries as the
// catalog_entry relation for Datalog (curated, stable column set).
const CatalogEntryView = "catalog_entry_edb"

// FactScoredRelation is the Datalog relation name for currently-valid facts with
// read-time decay scoring (age and a half-life-decayed worth). Join-only.
const FactScoredRelation = "fact_scored"

// FactScoredView is the SQL view exposing current facts with computed age and
// decayed worth as the fact_scored relation for Datalog.
const FactScoredView = "fact_scored_edb"

// reservedRelations is the set of relation names reserved for built-in EDB
// relations. Keep in sync with the datalog override map.
var reservedRelations = map[string]bool{
	CatalogEntryRelation: true,
	FactScoredRelation:   true,
}

// IsReservedRelation reports whether rel is a built-in relation name that
// cannot be used for user-asserted facts.
func IsReservedRelation(rel string) bool {
	return reservedRelations[rel]
}

// ReservedRelations returns the set of reserved built-in relation names.
func ReservedRelations() []string {
	names := make([]string, 0, len(reservedRelations))
	for r := range reservedRelations {
		names = append(names, r)
	}
	return names
}
