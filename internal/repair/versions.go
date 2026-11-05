package repair

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/database"
)

// versions renumbers the history of every resource in which two entries hold
// the same version. Observation ingest used to record every observed entry as
// version 1 (pudl-42w), so a resource observed in several states has several
// version-1 entries. The affected resource's entries are re-sequenced 1..N in
// import order; resources whose versions are already unique are not touched.
//
// It is opt-in, because historical versions are otherwise never renumbered:
// `--fix` applies it only when `--only versions` names it.
var versions = Repair{
	Name:      "versions",
	Condition: "resources with two or more entries holding the same version",
	OptIn:     true,
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		return resequenceDuplicateVersions(env, true)
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		return resequenceDuplicateVersions(env, dryRun)
	},
}

func resequenceDuplicateVersions(env *Env, dryRun bool) (Result, error) {
	all, err := env.DB.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 0})
	if err != nil {
		return Result{}, fmt.Errorf("query catalog: %w", err)
	}
	entries := all.Entries
	original := make([]database.CatalogEntry, len(entries))
	copy(original, entries)

	resequence(entries, duplicatedVersionResources(entries))

	var result Result
	for i := range entries {
		entry, old := entries[i], original[i]
		if equalPtr(entry.Version, old.Version) {
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
	result.Message = fmt.Sprintf("%d entries to renumber", result.Pending)
	return result, nil
}

// duplicatedVersionResources returns the resource_ids under which two entries
// hold the same version.
func duplicatedVersionResources(entries []database.CatalogEntry) map[string]bool {
	type key struct {
		resourceID string
		version    int
	}
	seen := map[key]bool{}
	duplicated := map[string]bool{}
	for _, e := range entries {
		if e.ResourceID == nil || e.Version == nil {
			continue
		}
		k := key{*e.ResourceID, *e.Version}
		if seen[k] {
			duplicated[k.resourceID] = true
		}
		seen[k] = true
	}
	return duplicated
}
