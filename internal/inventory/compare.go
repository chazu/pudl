package inventory

import (
	"context"
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
)

// Change is one resource that differs between two snapshots of a scope.
type Change struct {
	Resource string         `json:"resource"`
	Change   string         `json:"change"`
	Before   map[string]any `json:"before,omitempty"`
	After    map[string]any `json:"after,omitempty"`
}

// Comparison is what changed between two retained snapshots of one scope.
// Absence from an incomplete snapshot is reported as newly-observed or
// not-observed rather than added or removed.
type Comparison struct {
	Before            *database.ObserveSnapshot
	After             *database.ObserveSnapshot
	Changes           []Change
	IdentityFallbacks map[string][]string
}

// CompareSnapshots compares two retained snapshots of the same scope and
// owner by record identity. loadPolicy is called once both snapshots are
// found.
func CompareSnapshots(ctx context.Context, db *database.CatalogDB, before, after string, loadPolicy func() (Policy, error)) (*Comparison, error) {
	old, err := db.GetObserveSnapshot(before)
	if err != nil {
		return nil, err
	}
	current, err := db.GetObserveSnapshot(after)
	if err != nil {
		return nil, err
	}
	if old == nil || current == nil {
		return nil, fmt.Errorf("comparison requires two retained observation snapshots")
	}
	if old.Scope != current.Scope || old.Model != current.Model || old.Source != current.Source || old.Origin != current.Origin {
		return nil, fmt.Errorf("snapshots describe different observation scopes or owners")
	}
	policy, err := loadPolicy()
	if err != nil {
		return nil, err
	}
	var compared []acute.ObservedRecord
	index := func(id string) (map[string]acute.ObservedRecord, map[string]string, error) {
		set, err := LoadObserved(ctx, db, id)
		if err != nil {
			return nil, nil, err
		}
		byKey := map[string]acute.ObservedRecord{}
		labels := map[string]string{}
		for _, record := range set.Records {
			record.Data, err = NormalizeRecord(record.Data, policy.Namespace)
			if err != nil {
				return nil, nil, err
			}
			compared = append(compared, record)
			key, label, ok := acute.RecordIdentity(record.Data, policy.Identity)
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
		return nil, err
	}
	b, newLabels, err := index(after)
	if err != nil {
		return nil, err
	}
	for key, label := range newLabels {
		labels[key] = label
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	changes := []Change{}
	for _, key := range keys {
		oldRecord, had := a[key]
		newRecord, has := b[key]
		change := Change{Resource: labels[key], Before: oldRecord.Data, After: newRecord.Data}
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
	return &Comparison{
		Before: old, After: current, Changes: changes,
		IdentityFallbacks: acute.IdentityFallbacks(policy.Identity, nil, compared),
	}, nil
}
