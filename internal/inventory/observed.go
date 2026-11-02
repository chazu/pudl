// Package inventory compares catalog observations: a model's desired records
// against the records an observation snapshot holds (inventory drift), and
// two snapshots of the same scope against each other. It reads the catalog;
// the pure set-diff and field comparison live in internal/acute.
package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
)

// ScopeFilter turns a scope string and the result of looking it up as a
// snapshot collection into the filter the observe query should use: either a
// collection ID (the scope names a snapshot) or an origin (the compatibility
// path for explicit catalog callers and tests).
//
// Only a *not-found* may fall back to the origin filter. Any other lookup
// failure is fatal, because falling back on a genuine database error filters by
// an origin that matches nothing, hands the set-diff an empty observed set, and
// reports every desired resource `missing` — a transient DB fault rendered as a
// confident "everything is drifted".
func ScopeFilter(scope string, lookupErr error) (collectionID, origin string, err error) {
	switch {
	case lookupErr == nil:
		return scope, "", nil
	case errors.GetErrorCode(lookupErr) == errors.ErrCodeNotFound:
		return "", scope, nil
	default:
		return "", "", fmt.Errorf("resolve observe scope %q: %w", scope, lookupErr)
	}
}

// Observed is the inventory observation a drift verdict compares against: its
// records, and the snapshot that holds them when the scope named one.
type Observed struct {
	Complete   bool
	Records    []acute.ObservedRecord
	SnapshotID string
	ObservedAt *time.Time
}

// LoadObserved reads the observe records in scope from the catalog (by
// snapshot, or by origin), each with the time it was recorded. Numbers decode
// exactly so comparison against desired values does not round through
// float64.
func LoadObserved(ctx context.Context, db *database.CatalogDB, scope string) (Observed, error) {
	var set Observed
	filter := database.FilterOptions{EntryTypes: []string{"observe"}, CollectionType: "item"}
	// A snapshot ID is the normal scope for a live inventory run. Keep origin
	// filtering as a compatibility path for explicit catalog callers and tests.
	//
	// Only a *not-found* justifies that fallback — see ScopeFilter.
	if scope != "" {
		_, lookupErr := db.GetCollectionByID(scope)
		collectionID, origin, err := ScopeFilter(scope, lookupErr)
		if err != nil {
			return set, err
		}
		filter.CollectionID, filter.Origin = collectionID, origin
		if collectionID != "" {
			set.SnapshotID = collectionID
			snapshot, err := db.GetObserveSnapshot(collectionID)
			if err != nil {
				return set, err
			}
			if snapshot != nil && !snapshot.CreatedAt.IsZero() {
				set.Complete = snapshot.Complete
				at := snapshot.CreatedAt
				set.ObservedAt = &at
			}
		}
	}
	res, err := db.QueryEntriesContext(ctx, database.FilterOptions{
		EntryTypes:     filter.EntryTypes,
		CollectionType: filter.CollectionType,
		Origin:         filter.Origin,
		CollectionID:   filter.CollectionID,
	}, database.QueryOptions{})
	if err != nil {
		return set, fmt.Errorf("query observed records: %w", err)
	}
	for _, e := range res.Entries {
		if err := ctx.Err(); err != nil {
			return set, err
		}
		data, err := os.ReadFile(e.StoredPath)
		if err != nil {
			return set, fmt.Errorf("read observed record %s: %w", e.StoredPath, err)
		}
		var rec map[string]any
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&rec); err != nil {
			return set, fmt.Errorf("parse observed record %s: %w", e.StoredPath, err)
		}
		if tag, _ := rec["_schema"].(string); tag == "" && e.IdentityJSON != nil {
			rec["_schema"] = e.Schema
		}
		observed := acute.ObservedRecord{Data: rec, ObservedAt: set.ObservedAt}
		if e.IdentityJSON != nil && *e.IdentityJSON != "" {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(*e.IdentityJSON), &fields); err != nil {
				return set, fmt.Errorf("decode historical identity for %s: %w", e.ID, err)
			}
			for field := range fields {
				observed.IdentityFields = append(observed.IdentityFields, field)
			}
		}
		if !e.ImportTimestamp.IsZero() {
			at := e.ImportTimestamp
			observed.ObservedAt = &at
		}
		set.Records = append(set.Records, observed)
	}
	return set, nil
}
