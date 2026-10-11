package cmd

import (
	"context"
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
)

var snapshotCompare string

type snapshotChange struct {
	Resource string         `json:"resource"`
	Change   string         `json:"change"`
	Before   map[string]any `json:"before,omitempty"`
	After    map[string]any `json:"after,omitempty"`
}

func compareSnapshots(ctx context.Context, db *database.CatalogDB, before, after string) error {
	old, err := db.GetObserveSnapshot(before)
	if err != nil {
		return err
	}
	current, err := db.GetObserveSnapshot(after)
	if err != nil {
		return err
	}
	if old == nil || current == nil {
		return fmt.Errorf("comparison requires two retained observation snapshots")
	}
	if old.Scope != current.Scope || old.Model != current.Model || old.Source != current.Source || old.Origin != current.Origin {
		return fmt.Errorf("snapshots describe different observation scopes or owners")
	}
	identity, err := schemaIdentityResolver()
	if err != nil {
		return err
	}
	index := func(id string) (map[string]acute.ObservedRecord, map[string]string, error) {
		set, err := loadObservedRecordsContext(ctx, db, id)
		if err != nil {
			return nil, nil, err
		}
		byKey := map[string]acute.ObservedRecord{}
		labels := map[string]string{}
		for _, record := range set.records {
			key, label, ok := acute.RecordIdentity(record.Data, identity)
			if !ok {
				return nil, nil, fmt.Errorf("snapshot %s has unidentifiable records", id)
			}
			if prior, exists := byKey[key]; exists && !acute.ValuesEqual(prior.Data, record.Data) {
				return nil, nil, fmt.Errorf("snapshot %s has ambiguous identity %s", id, label)
			}
			byKey[key] = record
			labels[key] = label
		}
		return byKey, labels, nil
	}
	a, labels, err := index(before)
	if err != nil {
		return err
	}
	b, newLabels, err := index(after)
	if err != nil {
		return err
	}
	for key, label := range newLabels {
		labels[key] = label
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	changes := []snapshotChange{}
	for _, key := range keys {
		oldRecord, had := a[key]
		newRecord, has := b[key]
		change := snapshotChange{Resource: labels[key], Before: oldRecord.Data, After: newRecord.Data}
		switch {
		case !had:
			change.Change = "newly-observed"
			if old.Complete {
				change.Change = "added"
			}
		case !has:
			change.Change = "not-observed"
			if current.Complete {
				change.Change = "removed"
			}
		case !acute.ValuesEqual(oldRecord.Data, newRecord.Data):
			change.Change = "changed"
		default:
			continue
		}
		changes = append(changes, change)
	}
	if jsonOutput {
		return printJSON(map[string]any{"before_snapshot": old, "after_snapshot": current, "changes": changes})
	}
	fmt.Fprintf(outw(), "%s → %s: %d changes (complete: %t → %t)\n", before, after, len(changes), old.Complete, current.Complete)
	for _, change := range changes {
		fmt.Fprintf(outw(), "  %s: %s\n", change.Resource, change.Change)
	}
	return nil
}
