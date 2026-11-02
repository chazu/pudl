package repair

import (
	"context"
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/inference"
)

// identities backfills resource_id and identity_json for entries that predate
// identity tracking, and recomputes resource_ids still namespaced by their
// leaf schema instead of their inheritance-family root (formerly
// `pudl migrate identity` and `pudl migrate identity --recompute`).
//
// A stale identity is detected exactly: the stored resource_id equals the
// leaf-namespaced id and differs from the root-namespaced one. Family-root
// namespacing can merge previously distinct resources, so the versions of
// every resource a recomputed entry joins are re-sequenced into one history.
var identities = Repair{
	Name:      "identity",
	Condition: "entries with no resource_id, or one namespaced by the leaf schema rather than its family root",
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		return migrateIdentities(env, true)
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		return migrateIdentities(env, dryRun)
	},
}

func migrateIdentities(env *Env, dryRun bool) (Result, error) {
	all, err := env.DB.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 0})
	if err != nil {
		return Result{}, fmt.Errorf("query catalog: %w", err)
	}
	graph := env.Inferrer.GetInheritanceGraph()

	var result Result
	entries := all.Entries
	original := make([]database.CatalogEntry, len(entries))
	copy(original, entries)
	merged := map[string]bool{} // resource_ids recomputed entries now share
	missing, stale := 0, 0
	for i := range entries {
		entry := &entries[i]
		root := graph.IdentityRoot(entry.Schema)
		if entry.ResourceID != nil && root == entry.Schema {
			continue // a family root namespaces itself; nothing can be stale
		}
		values, err := loadIdentityValues(*entry, env.Inferrer)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", entry.ID, err))
			continue
		}
		if entry.ResourceID == nil {
			missing++
			applyIdentity(entry, root, values)
			continue
		}
		hash := contentHash(*entry)
		legacy := identity.ComputeResourceID(entry.Schema, values, hash)
		if *entry.ResourceID != legacy || legacy == identity.ComputeResourceID(root, values, hash) {
			continue
		}
		stale++
		applyIdentity(entry, root, values)
		merged[*entry.ResourceID] = true
	}
	resequence(entries, merged)

	for i := range entries {
		entry, old := entries[i], original[i]
		if sameIdentity(entry, old) {
			continue
		}
		result.Pending++
		result.Changes = append(result.Changes, Change{Subject: entry.ID, From: describeIdentity(old), To: describeIdentity(entry)})
		if dryRun {
			continue
		}
		if err := env.DB.UpdateEntry(entry); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", entry.ID, err))
			continue
		}
		result.Changed++
	}
	result.Message = fmt.Sprintf("%d entries without resource_id, %d with a leaf-namespaced resource_id", missing, stale)
	return result, nil
}

// resequence assigns one coherent version history to each resource in merged,
// across every entry that now carries its resource_id.
func resequence(entries []database.CatalogEntry, merged map[string]bool) {
	if len(merged) == 0 {
		return
	}
	var idx []int
	var group []database.CatalogEntry
	for i, e := range entries {
		if e.ResourceID != nil && merged[*e.ResourceID] {
			idx = append(idx, i)
			group = append(group, e)
		}
	}
	assignVersions(group)
	for j, i := range idx {
		entries[i].Version = group[j].Version
	}
}

func sameIdentity(a, b database.CatalogEntry) bool {
	return equalPtr(a.ResourceID, b.ResourceID) && equalPtr(a.IdentityJSON, b.IdentityJSON) && equalPtr(a.Version, b.Version)
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// loadIdentityValues extracts entry's identity values. Only schemas with
// identity fields need the payload; others are identified by content hash.
func loadIdentityValues(entry database.CatalogEntry, inferrer *inference.SchemaInferrer) (map[string]interface{}, error) {
	meta, found := inferrer.GetSchemaMetadata(entry.Schema)
	if !found || len(meta.IdentityFields) == 0 {
		return nil, nil
	}
	data, err := importer.DecodeStoredFile(entry.StoredPath, entry.Format)
	if err != nil {
		return nil, fmt.Errorf("cannot read data file: %w", err)
	}
	return identityValues(data, meta.IdentityFields), nil
}

func identityValues(data interface{}, fields []string) map[string]interface{} {
	values, err := identity.ExtractFieldValues(data, fields)
	if err != nil {
		return nil
	}
	return values
}

func contentHash(entry database.CatalogEntry) string {
	if entry.ContentHash != nil {
		return *entry.ContentHash
	}
	return entry.ID
}

// applyIdentity sets entry's resource_id and identity_json, namespaced by
// namespace (the family root of its schema).
func applyIdentity(entry *database.CatalogEntry, namespace string, values map[string]interface{}) {
	resourceID := identity.ComputeResourceID(namespace, values, contentHash(*entry))
	entry.ResourceID = &resourceID
	entry.IdentityJSON = nil
	if len(values) > 0 {
		if canonical, err := identity.CanonicalIdentityJSON(values); err == nil {
			entry.IdentityJSON = &canonical
		}
	}
}

// recomputeIdentity recomputes entry's identity under its current schema from
// already-loaded data. The version stays: reclassification is not a new
// observation.
func recomputeIdentity(entry *database.CatalogEntry, data interface{}, inferrer *inference.SchemaInferrer) {
	var fields []string
	if meta, found := inferrer.GetSchemaMetadata(entry.Schema); found {
		fields = meta.IdentityFields
	}
	applyIdentity(entry, inferrer.GetInheritanceGraph().IdentityRoot(entry.Schema), identityValues(data, fields))
}

// assignVersions groups entries by resource_id and assigns a monotonic version
// (1..N) within each group, ordered by import time (tie-broken by created time,
// then id) so the sequence is deterministic and idempotent.
func assignVersions(entries []database.CatalogEntry) {
	groups := make(map[string][]int)
	for i := range entries {
		if entries[i].ResourceID == nil {
			continue
		}
		rid := *entries[i].ResourceID
		groups[rid] = append(groups[rid], i)
	}

	for _, idxs := range groups {
		sort.SliceStable(idxs, func(a, b int) bool {
			ea, eb := entries[idxs[a]], entries[idxs[b]]
			if !ea.ImportTimestamp.Equal(eb.ImportTimestamp) {
				return ea.ImportTimestamp.Before(eb.ImportTimestamp)
			}
			if !ea.CreatedAt.Equal(eb.CreatedAt) {
				return ea.CreatedAt.Before(eb.CreatedAt)
			}
			return ea.ID < eb.ID
		})
		for v, idx := range idxs {
			version := v + 1
			entries[idx].Version = &version
		}
	}
}

// describeIdentity renders a resource_id prefix and version for reports.
func describeIdentity(entry database.CatalogEntry) string {
	if entry.ResourceID == nil {
		return "(none)"
	}
	rid := *entry.ResourceID
	if len(rid) > 16 {
		rid = rid[:16]
	}
	if entry.Version != nil {
		return fmt.Sprintf("%s v%d", rid, *entry.Version)
	}
	return rid
}
