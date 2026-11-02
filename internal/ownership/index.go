package ownership

import (
	"encoding/json"

	"github.com/chazu/pudl/internal/database"
)

// The built-in ownership relations schema projection writes for every
// resource whose type declares `_pudl.owner`: resource_owner when an owner
// was derived, resource_unowned otherwise. A facts block cannot declare them.
const (
	OwnerRelation   = "resource_owner"
	UnownedRelation = "resource_unowned"
)

// Attribution is one resource's current ownership fact.
type Attribution struct {
	ResourceID string `json:"resource_id"`
	EntryID    string `json:"entry_id"`
	Type       string `json:"type"`
	// Owner is "" when the resource is unowned.
	Owner string `json:"owner,omitempty"`
	Via   string `json:"via,omitempty"`
}

// Index resolves owners from the current resource_owner and resource_unowned
// facts. Resources whose type declares no owner are not in it.
type Index struct {
	db        *database.CatalogDB
	resources map[string]Attribution
	// entries caches catalog entry ID -> resource ID.
	entries map[string]string
}

// Load reads the current ownership facts of db.
func Load(db *database.CatalogDB) (*Index, error) {
	idx := &Index{db: db, resources: map[string]Attribution{}, entries: map[string]string{}}
	for _, relation := range []string{OwnerRelation, UnownedRelation} {
		facts, err := db.QueryCurrentFacts(relation)
		if err != nil {
			return nil, err
		}
		for _, f := range facts {
			var a Attribution
			if json.Unmarshal([]byte(f.Args), &a) != nil || a.ResourceID == "" {
				continue
			}
			idx.resources[a.ResourceID] = a
			idx.entries[a.EntryID] = a.ResourceID
		}
	}
	return idx, nil
}

// Resource returns a resource's attribution; ok is false when its type
// declares no owner.
func (i *Index) Resource(resourceID string) (Attribution, bool) {
	if i == nil {
		return Attribution{}, false
	}
	a, ok := i.resources[resourceID]
	return a, ok
}

// Entry returns the attribution of the resource a catalog entry records. An
// older entry of a resource resolves to the resource's current owner.
func (i *Index) Entry(entryID string) (Attribution, bool) {
	if i == nil || entryID == "" {
		return Attribution{}, false
	}
	resourceID, cached := i.entries[entryID]
	if !cached && i.db != nil {
		if entry, err := i.db.GetEntry(entryID); err == nil && entry.ResourceID != nil {
			resourceID = *entry.ResourceID
		}
		i.entries[entryID] = resourceID
	}
	return i.Resource(resourceID)
}

// OwnerOf returns the owner of the first of resourceIDs, then entryIDs, that
// the index knows, or "" when none is owned.
func (i *Index) OwnerOf(resourceIDs, entryIDs []string) string {
	for _, id := range resourceIDs {
		if a, ok := i.Resource(id); ok {
			return a.Owner
		}
	}
	for _, id := range entryIDs {
		if a, ok := i.Entry(id); ok {
			return a.Owner
		}
	}
	return ""
}
