package projection

import (
	"encoding/json"
	"fmt"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/sameresource"
)

// sameResourceKeyArgs are the args of same_resource_key beside the implicit
// entry_id and resource_id:
//
//	same_resource_key(kind, key)
//
// key is the canonical JSON of the key's parts (see sameresource.Spec.Keys).
var sameResourceKeyArgs = []string{sameresource.ArgKind, sameresource.ArgKey}

// sameResource returns the nearest `_pudl.same_resource` along chain (most
// specific first), or the error that disables projection when that
// declaration is invalid.
func (r *Registry) sameResource(chain []string) (*sameresource.Spec, error) {
	for _, name := range chain {
		meta, ok := r.src.GetSchemaMetadata(name)
		if !ok {
			continue
		}
		if meta.SameResourceError != "" {
			return nil, fmt.Errorf("schema %s: %s", name, meta.SameResourceError)
		}
		if meta.SameResource != nil {
			return meta.SameResource, nil
		}
	}
	return nil, nil
}

// chainDeclaresSameResource reports whether any schema along schema's chain
// declares `_pudl.same_resource`, valid or not.
func (r *Registry) chainDeclaresSameResource(schema string) bool {
	for _, name := range r.src.GetInheritanceGraph().GetCascadeChain(schema) {
		if meta, ok := r.src.GetSchemaMetadata(name); ok && (meta.SameResource != nil || meta.SameResourceError != "") {
			return true
		}
	}
	return false
}

// sameResourceKeyFacts are a record's same_resource_key facts, one per key.
func sameResourceKeyFacts(spec *SchemaSpec, entryID, resourceID string, data any) []database.Fact {
	var out []database.Fact
	for _, k := range spec.SameResource.Keys(data) {
		args := map[string]string{
			ArgEntryID: entryID, ArgResourceID: resourceID,
			sameresource.ArgKind: k.Kind, sameresource.ArgKey: k.Key,
		}
		encoded, _ := json.Marshal(args) // a map of strings always encodes
		out = append(out, database.Fact{
			Relation:   database.SameResourceKeyRelation,
			Args:       string(encoded),
			Provenance: fmt.Sprintf(`{"entry_id":%q,"schema":%q}`, entryID, spec.Schema),
		})
	}
	return out
}

// addSameResourceKeyArgs adds same_resource_key's args to out.
func addSameResourceKeyArgs(out map[string]map[string]bool) {
	args := out[database.SameResourceKeyRelation]
	if args == nil {
		args = map[string]bool{ArgEntryID: true, ArgResourceID: true}
		out[database.SameResourceKeyRelation] = args
	}
	for _, name := range sameResourceKeyArgs {
		args[name] = true
	}
}
