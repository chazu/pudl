package database

import (
	"sort"
	"time"

	"github.com/chazu/pudl/internal/errors"
)

// ResourceEntriesAsOf returns, for every resource with a resolved identity
// that had been recorded by t, its most recently recorded entry at t: the
// catalog as it stood then. Entries deleted since are gone and cannot be
// shown. Ordering is done in Go because import_timestamp is stored as text
// with a zone offset, which SQLite cannot compare reliably.
func (c *CatalogDB) ResourceEntriesAsOf(t time.Time) ([]CatalogEntry, error) {
	rows, err := c.db.Query(`SELECT ` + entrySelect("catalog_entries") + ` FROM catalog_entries
		WHERE resource_id IS NOT NULL AND identity_json IS NOT NULL AND identity_json != ''`)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeDatabaseError, "list resource entries", err)
	}
	defer rows.Close()
	latest := map[string]CatalogEntry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		if e.ImportTimestamp.After(t) {
			continue
		}
		if prev, ok := latest[*e.ResourceID]; !ok || e.ImportTimestamp.After(prev.ImportTimestamp) {
			latest[*e.ResourceID] = *e
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]CatalogEntry, 0, len(latest))
	for _, e := range latest {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return *out[i].ResourceID < *out[j].ResourceID })
	return out, nil
}
