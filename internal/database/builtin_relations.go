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

// Shared identity across schemas (see internal/sameresource). Only projection
// writes same_resource_key, from a schema's `_pudl.same_resource`; same_resource
// is only ever derived by rules.
const (
	SameResourceRelation    = "same_resource"
	SameResourceKeyRelation = "same_resource_key"
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

// sameResourceRelations are reserved for shared identity.
var sameResourceRelations = map[string]bool{
	SameResourceRelation:    true,
	SameResourceKeyRelation: true,
}

// IsReservedRelation reports whether rel is a built-in relation name that
// cannot be used for user-asserted facts.
func IsReservedRelation(rel string) bool {
	return reservedRelations[rel] || policyRelations[rel] || sameResourceRelations[rel]
}

// ReservedRelations returns the set of reserved built-in relation names.
func ReservedRelations() []string {
	names := make([]string, 0, len(reservedRelations)+len(policyRelations)+len(sameResourceRelations))
	for _, set := range []map[string]bool{reservedRelations, policyRelations, sameResourceRelations} {
		for r := range set {
			names = append(names, r)
		}
	}
	return names
}

// checkReservedRelation rejects a fact under a reserved relation, except a
// policy result written by policy evaluation under its own source and a
// same_resource_key fact written by projection.
func checkReservedRelation(f Fact) error {
	switch {
	case policyRelations[f.Relation] && IsPolicySource(f.Source):
		return nil
	case f.Relation == SameResourceKeyRelation && IsProjectionSource(f.Source):
		return nil
	case f.Relation == SameResourceKeyRelation:
		return errors.WrapError(errors.ErrCodeInvalidInput,
			fmt.Sprintf("relation %q is reserved for keys projected from a schema's _pudl.same_resource and cannot be used for facts", f.Relation), nil)
	case f.Relation == SameResourceRelation:
		return errors.WrapError(errors.ErrCodeInvalidInput,
			fmt.Sprintf("relation %q is derived by rules only (see same_resource_by_key) and cannot be used for facts", f.Relation), nil)
	case policyRelations[f.Relation]:
		return errors.WrapError(errors.ErrCodeInvalidInput,
			fmt.Sprintf("relation %q is reserved for compliance policy results (see pudl policy evaluate) and cannot be used for facts", f.Relation), nil)
	case reservedRelations[f.Relation]:
		return errors.WrapError(errors.ErrCodeInvalidInput,
			fmt.Sprintf("relation %q is reserved for the built-in catalog EDB and cannot be used for facts", f.Relation), nil)
	}
	return nil
}
