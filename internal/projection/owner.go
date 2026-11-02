package projection

import (
	"encoding/json"
	"fmt"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/ownership"
)

// Args of the built-in ownership relations, beside the implicit entry_id and
// resource_id:
//
//	resource_owner(type, owner, via)
//	resource_unowned(type)
//
// type is the record's schema; via says how the owner was derived
// ("path:<path>", "map:<path>" or "fallback").
const (
	ArgType  = "type"
	ArgOwner = "owner"
	ArgVia   = "via"
)

// ownerRelationArgs are the args each ownership relation carries.
var ownerRelationArgs = map[string][]string{
	ownership.OwnerRelation:   {ArgType, ArgOwner, ArgVia},
	ownership.UnownedRelation: {ArgType},
}

// owner returns the nearest `_pudl.owner` along chain (most specific first),
// or the error that disables projection when that declaration is invalid.
func (r *Registry) owner(chain []string) (*ownership.Spec, error) {
	for _, name := range chain {
		meta, ok := r.src.GetSchemaMetadata(name)
		if !ok {
			continue
		}
		if meta.OwnerError != "" {
			return nil, fmt.Errorf("schema %s: %s", name, meta.OwnerError)
		}
		if meta.Owner != nil {
			return meta.Owner, nil
		}
	}
	return nil, nil
}

// chainDeclaresOwner reports whether any schema along schema's chain declares
// `_pudl.owner`, valid or not.
func (r *Registry) chainDeclaresOwner(schema string) bool {
	for _, name := range r.src.GetInheritanceGraph().GetCascadeChain(schema) {
		if meta, ok := r.src.GetSchemaMetadata(name); ok && (meta.Owner != nil || meta.OwnerError != "") {
			return true
		}
	}
	return false
}

// declaredWhat names what a spec projects, for error messages.
func declaredWhat(spec *SchemaSpec, relations map[string]RelationSpec) string {
	switch {
	case spec.Owner != nil && len(relations) > 0:
		return "facts and an owner"
	case spec.Owner != nil:
		return "an owner"
	default:
		return "facts"
	}
}

// ownerFact is the one ownership fact of a record: resource_owner when its
// spec names an owner, resource_unowned otherwise.
func ownerFact(spec *SchemaSpec, entryID, resourceID string, data any) database.Fact {
	args := map[string]string{ArgEntryID: entryID, ArgResourceID: resourceID, ArgType: spec.Schema}
	relation := ownership.UnownedRelation
	if owner, via, ok := spec.Owner.Resolve(data); ok {
		relation = ownership.OwnerRelation
		args[ArgOwner], args[ArgVia] = owner, via
	}
	encoded, _ := json.Marshal(args) // a map of strings always encodes
	return database.Fact{
		Relation:   relation,
		Args:       string(encoded),
		Provenance: fmt.Sprintf(`{"entry_id":%q,"schema":%q}`, entryID, spec.Schema),
	}
}

// addOwnerRelationArgs adds the ownership relations' args to out.
func addOwnerRelationArgs(out map[string]map[string]bool) {
	for rel, names := range ownerRelationArgs {
		args := out[rel]
		if args == nil {
			args = map[string]bool{ArgEntryID: true, ArgResourceID: true}
			out[rel] = args
		}
		for _, name := range names {
			args[name] = true
		}
	}
}
