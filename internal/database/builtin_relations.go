package database

import (
	"fmt"

	"github.com/chazu/pudl/internal/errors"
)

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

// Compliance policy results. Only policy evaluation writes them, under a
// PolicySource; they are ordinary facts to the Datalog engine.
const (
	PolicyStatusRelation    = "policy_status"
	PolicyViolationRelation = "policy_violation"
)

// reservedRelations is the set of relation names reserved for built-in EDB
// relations. Keep in sync with the datalog override map.
var reservedRelations = map[string]bool{
	CatalogEntryRelation: true,
}

// policyRelations are reserved for policy evaluation.
var policyRelations = map[string]bool{
	PolicyStatusRelation:    true,
	PolicyViolationRelation: true,
}

// IsReservedRelation reports whether rel is a built-in relation name that
// cannot be used for user-asserted facts.
func IsReservedRelation(rel string) bool {
	return reservedRelations[rel] || policyRelations[rel]
}

// ReservedRelations returns the set of reserved built-in relation names.
func ReservedRelations() []string {
	names := make([]string, 0, len(reservedRelations)+len(policyRelations))
	for _, set := range []map[string]bool{reservedRelations, policyRelations} {
		for r := range set {
			names = append(names, r)
		}
	}
	return names
}

// checkReservedRelation rejects a fact under a reserved relation, except a
// policy result written by policy evaluation under its own source.
func checkReservedRelation(f Fact) error {
	switch {
	case policyRelations[f.Relation] && IsPolicySource(f.Source):
		return nil
	case policyRelations[f.Relation]:
		return errors.WrapError(errors.ErrCodeInvalidInput,
			fmt.Sprintf("relation %q is reserved for compliance policy results (see pudl policy evaluate) and cannot be used for facts", f.Relation), nil)
	case reservedRelations[f.Relation]:
		return errors.WrapError(errors.ErrCodeInvalidInput,
			fmt.Sprintf("relation %q is reserved for the built-in catalog EDB and cannot be used for facts", f.Relation), nil)
	}
	return nil
}
