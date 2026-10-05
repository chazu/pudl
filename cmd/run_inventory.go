package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/inference"
)

// identityResolver returns the declared identity_fields for a schema, or nil when
// the schema is unknown or declares none. In the run path it is backed by the
// inference graph (see schemaIdentityResolver); tests inject a stub.
type identityResolver = acute.IdentityResolver

// modelResourceDefs returns the candidate catalog definition names for a model's
// desired resources — the bare resource names that `ingest-manifest` keys
// per-resource status on (targetToDefinition(action.Target)). Each desired record
// contributes the values of its declared identity_fields, else the first present
// of name | path | id. The set is a superset used only to scope a converging->clean
// promotion, so extra candidates that match nothing are harmless.
func modelResourceDefs(desired []map[string]any, identity identityResolver) []string {
	seen := map[string]bool{}
	var defs []string
	add := func(v any) {
		s := fmt.Sprintf("%v", v)
		if s != "" && !seen[s] {
			seen[s] = true
			defs = append(defs, s)
		}
	}
	for _, d := range desired {
		schema, _ := d["_schema"].(string)
		matched := false
		if identity != nil {
			for _, f := range identity(schema) {
				if v, ok := d[f]; ok {
					add(v)
					matched = true
				}
			}
		}
		if !matched {
			for _, k := range []string{"name", "path", "id"} {
				if v, ok := d[k]; ok {
					add(v)
					break
				}
			}
		}
	}
	return defs
}

// schemaIdentityResolver builds an identityResolver backed by the schema inferrer:
// each schema's declared identity_fields from the inference graph. Records whose
// schema is unknown or declares none fall back to the name|path|id heuristic.
func schemaIdentityResolver() (identityResolver, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inferrer, err := inference.Shared(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("init schema inferrer: %w", err)
	}
	return func(schema string) []string {
		if meta, ok := inferrer.GetSchemaMetadata(schema); ok {
			return meta.IdentityFields
		}
		return nil
	}, nil
}

// observeScopeFilter turns a scope string and the result of looking it up as a
// snapshot collection into the filter the observe query should use: either a
// collection ID (the scope names a snapshot) or an origin (the compatibility
// path for explicit catalog callers and tests).
//
// Only a *not-found* may fall back to the origin filter. Any other lookup
// failure is fatal, because falling back on a genuine database error filters by
// an origin that matches nothing, hands the set-diff an empty observed set, and
// reports every desired resource `missing` — a transient DB fault rendered as a
// confident "everything is drifted", which under --converge means re-applying
// the entire model.
func observeScopeFilter(scope string, lookupErr error) (collectionID, origin string, err error) {
	switch {
	case lookupErr == nil:
		return scope, "", nil
	case errors.GetErrorCode(lookupErr) == errors.ErrCodeNotFound:
		return "", scope, nil
	default:
		return "", "", fmt.Errorf("resolve observe scope %q: %w", scope, lookupErr)
	}
}

// observedSet is the inventory observation a drift verdict compares against:
// its records, and the snapshot that holds them when the scope named one.
type observedSet struct {
	records    []acute.ObservedRecord
	snapshotID string
	observedAt *time.Time
}

// loadObservedRecords reads the inventory records ingested for this run from the
// catalog (observe items by snapshot or origin), each with the time it was
// recorded. Numbers decode exactly so comparison against desired values does
// not round through float64.
func loadObservedRecords(db *database.CatalogDB, scope string) (observedSet, error) {
	var set observedSet
	filter := database.FilterOptions{EntryTypes: []string{"observe"}, CollectionType: "item"}
	// A snapshot ID is the normal scope for a live inventory run. Keep origin
	// filtering as a compatibility path for explicit catalog callers and tests.
	//
	// Only a *not-found* justifies that fallback — see observeScopeFilter.
	if scope != "" {
		_, lookupErr := db.GetCollectionByID(scope)
		collectionID, origin, err := observeScopeFilter(scope, lookupErr)
		if err != nil {
			return set, err
		}
		filter.CollectionID, filter.Origin = collectionID, origin
		if collectionID != "" {
			set.snapshotID = collectionID
			snapshot, err := db.GetObserveSnapshot(collectionID)
			if err != nil {
				return set, err
			}
			if snapshot != nil && !snapshot.CreatedAt.IsZero() {
				at := snapshot.CreatedAt
				set.observedAt = &at
			}
		}
	}
	res, err := db.QueryEntries(database.FilterOptions{
		EntryTypes:     filter.EntryTypes,
		CollectionType: filter.CollectionType,
		Origin:         filter.Origin,
		CollectionID:   filter.CollectionID,
	}, database.QueryOptions{})
	if err != nil {
		return set, fmt.Errorf("query observed records: %w", err)
	}
	for _, e := range res.Entries {
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
		observed := acute.ObservedRecord{Data: rec, ObservedAt: set.observedAt}
		if !e.ImportTimestamp.IsZero() {
			at := e.ImportTimestamp
			observed.ObservedAt = &at
		}
		set.records = append(set.records, observed)
	}
	return set, nil
}

// runInventoryDrift computes drift for an inventory model: desired vs the
// observed records in the catalog (set-diff by identity). For inventory
// observers (host) that dump records — distinct from the differential path
// (k8s), where the plugin does the diff.
//
// The scope is required. Without one the query returns every observation in the
// catalog — every model, every host, all time — so a desired record could be
// satisfied by an unrelated model's observation and the run could report clean
// against records it has nothing to do with. Callers must pass either the
// snapshot they just populated or an explicitly requested replay scope.
//
// Verified is deliberately left false here: this function cannot tell whether
// its scope names a fresh snapshot or a stale replay, so the caller that chose
// the scope is the one that gets to claim verification.
func runInventoryDrift(db *database.CatalogDB, scope string, desired []map[string]any, identity identityResolver) (ModelDriftResult, error) {
	if strings.TrimSpace(scope) == "" {
		return ModelDriftResult{}, fmt.Errorf("inventory drift requires a catalog scope (snapshot ID or origin); refusing to compare against every observation in the catalog")
	}
	observed, err := loadObservedRecords(db, scope)
	if err != nil {
		return ModelDriftResult{}, err
	}
	drifted := acute.InventorySetDiff(desired, observed.records, identity)
	return ModelDriftResult{
		Clean:      len(drifted) == 0,
		Drifted:    drifted,
		SnapshotID: observed.snapshotID,
		ObservedAt: observed.observedAt,
	}, nil
}
