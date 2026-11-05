// Package resourceref resolves values that name a cataloged resource (its
// resource ID, or the value of its single identity field, such as a GCP
// selfLink or a service account key name) to the resource, and describes the
// resource by its graph account (GCP project, AWS account) and type. Check
// reports use it to group findings whose arguments carry no resource ID.
package resourceref

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/graphspec"
	"github.com/chazu/pudl/internal/graphvocab"
)

// LoadFunc decodes an entry's stored (already redacted) payload.
type LoadFunc func(entry database.CatalogEntry) (any, error)

// preferredArgs are tried first, in order; other arguments follow by name.
var preferredArgs = []string{"resource_id", "resource", "target", "name"}

// Index resolves values to the catalog's current resources.
type Index struct {
	byValue   map[string]string // value -> resource ID; "" when ambiguous
	entries   map[string]database.CatalogEntry
	meta      graphvocab.MetadataFunc
	load      LoadFunc
	described map[string]checkreport.Description
}

// Load indexes the current entry of every resource.
func Load(db *database.CatalogDB, meta graphvocab.MetadataFunc, load LoadFunc) (*Index, error) {
	entries, err := db.CurrentResourceEntries()
	if err != nil {
		return nil, err
	}
	x := &Index{byValue: map[string]string{}, entries: map[string]database.CatalogEntry{}, meta: meta, load: load, described: map[string]checkreport.Description{}}
	for _, e := range entries {
		if e.ResourceID == nil || *e.ResourceID == "" {
			continue
		}
		id := *e.ResourceID
		x.entries[id] = e
		x.add(id, id)
		if v, ok := singleIdentityValue(e); ok {
			x.add(v, id)
		}
	}
	return x, nil
}

// add maps value to id; a value two resources share names neither.
func (x *Index) add(value, id string) {
	if prev, seen := x.byValue[value]; seen && prev != id {
		x.byValue[value] = ""
		return
	}
	x.byValue[value] = id
}

// singleIdentityValue is the value of an entry's identity when it has one
// field: multi-field identities are not a single value another record holds.
func singleIdentityValue(e database.CatalogEntry) (string, bool) {
	if e.IdentityJSON == nil {
		return "", false
	}
	var values map[string]any
	if json.Unmarshal([]byte(*e.IdentityJSON), &values) != nil || len(values) != 1 {
		return "", false
	}
	for _, v := range values {
		s, ok := graphspec.Scalar(v)
		return s, ok && s != ""
	}
	return "", false
}

// Resolve returns the resource a string argument names. When several do, it
// takes the most specific: the value that contains every other identifying
// value (a key's name contains its service account's email). Otherwise the
// first in order: resource_id, resource, target and name, then the others by
// name.
func (x *Index) Resolve(args map[string]any) (resourceID, value string, ok bool) {
	if x == nil {
		return "", "", false
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, pj := rank(keys[i]), rank(keys[j])
		if pi != pj {
			return pi < pj
		}
		return keys[i] < keys[j]
	})
	var found []candidate
	for _, k := range keys {
		s, isString := args[k].(string)
		if !isString || s == "" {
			continue
		}
		if id := x.byValue[s]; id != "" {
			found = append(found, candidate{id, s})
		}
	}
	if len(found) == 0 {
		return "", "", false
	}
	for _, c := range found {
		if containsAll(c.value, found) {
			return c.id, c.value, true
		}
	}
	return found[0].id, found[0].value, true
}

// candidate is a resource an argument value identifies.
type candidate struct{ id, value string }

// containsAll reports whether value contains every candidate's value.
func containsAll(value string, found []candidate) bool {
	for _, c := range found {
		if !strings.Contains(value, c.value) {
			return false
		}
	}
	return true
}

func rank(key string) int {
	for i, k := range preferredArgs {
		if k == key {
			return i
		}
	}
	return len(preferredArgs)
}

// Describe returns the resource's graph account (from its schema's
// `_pudl.graph.account`), resource type and name (its graph name, else a
// top-level name field, else its identity value). An unreadable payload
// yields no account and only the identity value as name.
func (x *Index) Describe(resourceID string) checkreport.Description {
	if x == nil {
		return checkreport.Description{}
	}
	if d, ok := x.described[resourceID]; ok {
		return d
	}
	e, ok := x.entries[resourceID]
	if !ok {
		return checkreport.Description{}
	}
	var d checkreport.Description
	if m, ok := x.meta(e.Schema); ok {
		d.Type = m.ResourceType
	}
	if x.load != nil {
		if record, err := x.load(e); err == nil {
			if spec := graphvocab.SpecFor(e.Schema, x.meta); spec != nil {
				d.Account = graphspec.Eval(spec.Account, record)
				d.Name = graphspec.Eval(spec.Name, record)
			}
			if d.Name == "" {
				if obj, ok := record.(map[string]any); ok {
					d.Name, _ = obj["name"].(string)
				}
			}
		}
	}
	if d.Name == "" {
		d.Name, _ = singleIdentityValue(e)
	}
	x.described[resourceID] = d
	return d
}
